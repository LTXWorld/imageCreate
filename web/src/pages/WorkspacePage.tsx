import { FormEvent, useEffect, useState } from "react";

import {
  api,
  generationImageFilename,
  normalizeGenerationTask,
  type GenerationTask,
  type User,
} from "../api/client";

const ratios = ["1:1", "3:4", "4:3", "9:16", "16:9"];
const activeStatuses = new Set<GenerationTask["status"]>(["queued", "running"]);
const safeFailureCodes = new Set(["content_rejected", "rate_limited", "timeout", "upstream_error"]);
const taskPollingIntervalMS = 5000;

type WorkspacePageProps = {
  user: User;
  onHistoryClick?: () => void;
};

function isActiveTask(task: GenerationTask | null) {
  return task ? activeStatuses.has(task.status) : false;
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

export function WorkspacePage({ user, onHistoryClick }: WorkspacePageProps) {
  const [prompt, setPrompt] = useState("");
  const [ratio, setRatio] = useState("1:1");
  const [submitting, setSubmitting] = useState(false);
  const [currentTask, setCurrentTask] = useState<GenerationTask | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!currentTask || !activeStatuses.has(currentTask.status)) return undefined;

    const taskId = currentTask.id;
    const timer = window.setInterval(() => {
      api<unknown>(`/api/generations/${taskId}`)
        .then((body) => setCurrentTask(normalizeGenerationTask(body as Parameters<typeof normalizeGenerationTask>[0])))
        .catch((err) => {
          setError(err instanceof Error ? err.message : "查询任务失败");
        });
    }, taskPollingIntervalMS);

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

              {isActiveTask(currentTask) ? <p className="muted-text">生成中</p> : null}
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
