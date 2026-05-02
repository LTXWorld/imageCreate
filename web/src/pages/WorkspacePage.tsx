import { FormEvent, useEffect, useState } from "react";

import {
  api,
  generationImageFilename,
  normalizeGenerationTask,
  type GenerationTask,
  type User,
} from "../api/client";

const ratios = ["1:1", "3:4", "4:3", "9:16", "16:9"];
const privateSupportConfig = {
  qq: import.meta.env.VITE_PRIVATE_SUPPORT_QQ?.trim() ?? "",
  wechat: import.meta.env.VITE_PRIVATE_SUPPORT_WECHAT?.trim() ?? "",
};
const activeStatuses = new Set<GenerationTask["status"]>(["queued", "running"]);
const safeFailureCodes = new Set(["content_rejected", "rate_limited", "timeout", "upstream_error"]);
const taskPollingIntervalMS = 5000;
const progressTickIntervalMS = 1000;
const queuedProgressDurationMS = 90_000;
const runningProgressDurationMS = 180_000;

type GenerationProgressState = {
  percent: number;
  label: string;
  helperText: string;
};

type WorkspacePageProps = {
  user: User;
  onHistoryClick?: () => void;
  onUserRefresh?: () => void | Promise<unknown>;
};

function isActiveTask(task: GenerationTask | null) {
  return task ? activeStatuses.has(task.status) : false;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function elapsedTaskMilliseconds(task: GenerationTask, now: number) {
  const createdAt = Date.parse(task.createdAt);
  if (Number.isNaN(createdAt)) return 0;
  return Math.max(0, now - createdAt);
}

function progressBetween(elapsedMS: number, durationMS: number, start: number, end: number) {
  const ratio = clamp(elapsedMS / durationMS, 0, 1);
  return Math.round(start + (end - start) * ratio);
}

function getGenerationProgress(task: GenerationTask, now: number): GenerationProgressState {
  const elapsedMS = elapsedTaskMilliseconds(task, now);

  if (task.status === "succeeded") {
    return {
      percent: 100,
      label: "生成完成",
      helperText: "图片已生成，可以预览或下载。",
    };
  }

  if (task.status === "failed") {
    return {
      percent: progressBetween(elapsedMS, runningProgressDurationMS, 25, 92),
      label: "生成未完成",
      helperText: "本次生成已结束，点数会按失败规则退回。",
    };
  }

  if (task.status === "running") {
    return {
      percent: progressBetween(elapsedMS, runningProgressDurationMS, 25, 92),
      label: elapsedMS > 120_000 ? "即将完成" : "正在绘制细节",
      helperText: "请保持页面打开，完成后会自动显示结果。",
    };
  }

  return {
    percent: progressBetween(elapsedMS, queuedProgressDurationMS, 5, 25),
    label: "正在排队",
    helperText: "任务已提交，系统正在准备生成。",
  };
}

function statusText(status: GenerationTask["status"]) {
  if (status === "queued" || status === "running") return "生成中";
  if (status === "succeeded") return "已完成";
  if (status === "failed") return "生成失败";
  return "已取消";
}

function safeFailureDetail(task: GenerationTask) {
  return task.errorCode && task.message && safeFailureCodes.has(task.errorCode)
    ? task.message
    : "";
}

function GenerationProgress({ task, now }: { task: GenerationTask; now: number }) {
  const progress = getGenerationProgress(task, now);

  return (
    <div className="generation-progress" aria-label="生成进度详情">
      <div className="progress-row">
        <strong>{progress.label}</strong>
        <span>{progress.percent}%</span>
      </div>
      <div
        aria-label="生成进度"
        aria-valuemax={100}
        aria-valuemin={0}
        aria-valuenow={progress.percent}
        className="progress-track"
        role="progressbar"
      >
        <span className="progress-fill" style={{ width: `${progress.percent}%` }} />
      </div>
      <p className="muted-text">{progress.helperText}</p>
    </div>
  );
}

function PrivateSupportCard() {
  const hasQQ = privateSupportConfig.qq.length > 0;
  const hasWechat = privateSupportConfig.wechat.length > 0;

  return (
    <section className="private-support" aria-label="专属服务">
      <div>
        <p className="eyebrow">专属服务</p>
        <h3>加 QQ 或微信获取帮助</h3>
        <p className="support-copy">额度咨询、生成失败处理、低价AI会员独享账号购买请直接与下方联系。</p>
      </div>
      <dl className="support-list">
        <div>
          <dt>QQ</dt>
          <dd>{hasQQ ? privateSupportConfig.qq : "待配置"}</dd>
        </div>
        <div>
          <dt>微信</dt>
          <dd>{hasWechat ? privateSupportConfig.wechat : "待配置"}</dd>
        </div>
      </dl>
    </section>
  );
}

export function WorkspacePage({ user, onHistoryClick, onUserRefresh }: WorkspacePageProps) {
  const [prompt, setPrompt] = useState("");
  const [ratio, setRatio] = useState("1:1");
  const [submitting, setSubmitting] = useState(false);
  const [currentTask, setCurrentTask] = useState<GenerationTask | null>(null);
  const [progressNow, setProgressNow] = useState(() => Date.now());
  const [error, setError] = useState("");

  function refreshUserCredits() {
    void Promise.resolve(onUserRefresh?.()).catch((err) => {
      setError(err instanceof Error ? err.message : "刷新额度失败");
    });
  }

  useEffect(() => {
    if (!currentTask || !activeStatuses.has(currentTask.status)) return undefined;

    const taskId = currentTask.id;
    const timer = window.setInterval(() => {
      api<unknown>(`/api/generations/${taskId}`)
        .then((body) => {
          const nextTask = normalizeGenerationTask(body as Parameters<typeof normalizeGenerationTask>[0]);
          setCurrentTask(nextTask);
          if (!activeStatuses.has(nextTask.status)) {
            refreshUserCredits();
          }
        })
        .catch((err) => {
          setError(err instanceof Error ? err.message : "查询任务失败");
        });
    }, taskPollingIntervalMS);

    return () => window.clearInterval(timer);
  }, [currentTask, onUserRefresh]);

  useEffect(() => {
    if (!currentTask || !activeStatuses.has(currentTask.status)) return undefined;

    setProgressNow(Date.now());
    const timer = window.setInterval(() => {
      setProgressNow(Date.now());
    }, progressTickIntervalMS);

    return () => window.clearInterval(timer);
  }, [currentTask]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmedPrompt = prompt.trim();
    if (!trimmedPrompt) return;

    setError("");
    setSubmitting(true);

    try {
      const body = await api<unknown>("/api/generations", {
        method: "POST",
        body: JSON.stringify({ prompt: trimmedPrompt, ratio }),
      });
      setCurrentTask(normalizeGenerationTask(body as Parameters<typeof normalizeGenerationTask>[0]));
      refreshUserCredits();
    } catch (err) {
      setError(err instanceof Error ? err.message : "提交生成失败");
    } finally {
      setSubmitting(false);
    }
  }

  const disabled = submitting || isActiveTask(currentTask);
  const failureDetail = currentTask ? safeFailureDetail(currentTask) : "";

  return (
    <section className="workspace-page" aria-labelledby="workspace-title">
      <div className="section-toolbar">
        <div className="section-heading">
          <p className="eyebrow">创作台</p>
          <h2 id="workspace-title">图像生成</h2>
        </div>
        <button className="secondary-button" type="button" onClick={onHistoryClick}>
          历史记录
        </button>
      </div>

      <div className="workspace-grid">
        <form className="generator-form panel" onSubmit={handleSubmit}>
          <div className="balance-row">
            <span>当前余额</span>
            <strong>{user.creditBalance} 点</strong>
          </div>
          <div className="balance-row">
            <span>今日免费额度 {user.dailyFreeCreditBalance}/{user.dailyFreeCreditLimit}</span>
            <span>付费额度 {user.paidCreditBalance}</span>
          </div>
          <p className="usage-note">
            输入提示词，选择画面比例后开始生成。每次生成 1 张图，扣 1 点；失败会自动退回点数。生成图片保留 30 天。
          </p>

          <label className="field">
            <span>提示词</span>
            <textarea
              name="prompt"
              onChange={(event) => setPrompt(event.target.value)}
              placeholder="描述你想生成的画面"
              required
              rows={6}
              value={prompt}
            />
          </label>

          <fieldset className="ratio-control">
            <legend>画面比例</legend>
            <div className="segmented-control">
              {ratios.map((item) => (
                <button
                  aria-pressed={ratio === item}
                  className={ratio === item ? "segment active" : "segment"}
                  key={item}
                  onClick={() => setRatio(item)}
                  type="button"
                >
                  {item}
                </button>
              ))}
            </div>
          </fieldset>

          {error ? <p className="form-error" role="alert">{error}</p> : null}

          <button className="primary-button wide-button" disabled={disabled} type="submit">
            {submitting ? "提交中..." : "生成"}
          </button>

          <PrivateSupportCard />
        </form>

        <section className="current-task panel" aria-label="当前任务">
          <div className="task-header">
            <div>
              <p className="eyebrow">当前任务</p>
              <h3>{currentTask ? statusText(currentTask.status) : "等待提交"}</h3>
            </div>
            {currentTask ? <span className={`status-badge ${currentTask.status}`}>{statusText(currentTask.status)}</span> : null}
          </div>

          {currentTask ? (
            <div className="task-detail">
              <p className="task-prompt">{currentTask.prompt}</p>
              <dl className="meta-list">
                <div>
                  <dt>比例</dt>
                  <dd>{currentTask.ratio}</dd>
                </div>
                <div>
                  <dt>尺寸</dt>
                  <dd>{currentTask.size}</dd>
                </div>
              </dl>

              {currentTask.status !== "canceled" ? <GenerationProgress task={currentTask} now={progressNow} /> : null}
              {currentTask.status === "failed" ? (
                <>
                  <p className="form-error" role="alert">
                    生成失败，已退回 1 点，可调整提示词后重试。
                  </p>
                  {failureDetail ? <p className="muted-text">{failureDetail}</p> : null}
                </>
              ) : null}
              {currentTask.status === "succeeded" && currentTask.imageUrl ? (
                <>
                  <img className="result-preview" src={currentTask.imageUrl} alt={currentTask.prompt} />
                  <a
                    className="secondary-button download-button"
                    download={generationImageFilename(currentTask)}
                    href={currentTask.imageUrl}
                  >
                    下载图片
                  </a>
                </>
              ) : null}
            </div>
          ) : (
            <div className="empty-state">填写提示词后开始生成。</div>
          )}
        </section>
      </div>
    </section>
  );
}
