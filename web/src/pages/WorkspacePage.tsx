import { ChangeEvent, FormEvent, useEffect, useState } from "react";
import {
  api,
  generationApi,
  generationImageFilename,
  normalizeGenerationTask,
  type GenerationTask,
  type User,
} from "../api/client";
import { ImagePreviewDialog } from "../components/ImagePreviewDialog";
import { PrivateSupportCard } from "../components/PrivateSupportCard";
import "../styles/Workspace.css";
import "../styles/Components.css";

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
const promptMaxLength = 2000;
const maxReferenceImageBytes = 5 * 1024 * 1024;
const acceptedReferenceImageTypes = new Set(["image/png", "image/jpeg"]);

type GenerationMode = "text" | "image";

type GenerationProgressState = {
  percent: number;
  label: string;
  helperText: string;
};

type PreviewImage = {
  alt: string;
  src: string;
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

function promptLength(value: string) {
  return Array.from(value.trim()).length;
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
      helperText: "已开始生成，请保持页面打开，完成后会自动显示结果。",
    };
  }

  return {
    percent: progressBetween(elapsedMS, queuedProgressDurationMS, 5, 25),
    label: "正在排队",
    helperText: "任务已提交，短时间内可取消本次提交。",
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

export function WorkspacePage({ user, onHistoryClick, onUserRefresh }: WorkspacePageProps) {
  const [prompt, setPrompt] = useState("");
  const [ratio, setRatio] = useState("1:1");
  const [generationMode, setGenerationMode] = useState<GenerationMode>("text");
  const [referenceImage, setReferenceImage] = useState<File | null>(null);
  const [referencePreviewURL, setReferencePreviewURL] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [canceling, setCanceling] = useState(false);
  const [currentTask, setCurrentTask] = useState<GenerationTask | null>(null);
  const [progressNow, setProgressNow] = useState(() => Date.now());
  const [previewImage, setPreviewImage] = useState<PreviewImage | null>(null);
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

  useEffect(() => {
    return () => {
      if (referencePreviewURL) {
        URL.revokeObjectURL(referencePreviewURL);
      }
    };
  }, [referencePreviewURL]);

  function clearReferenceImage() {
    setReferenceImage(null);
    setReferencePreviewURL("");
  }

  function handleReferenceImageChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0] ?? null;
    setError("");
    if (!file) {
      clearReferenceImage();
      return;
    }

    setReferenceImage(file);
    setReferencePreviewURL((currentURL) => {
      if (currentURL) URL.revokeObjectURL(currentURL);
      return URL.createObjectURL(file);
    });
  }

  function selectGenerationMode(nextMode: GenerationMode) {
    setGenerationMode(nextMode);
    setError("");
    if (nextMode === "text") {
      clearReferenceImage();
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    const currentPrompt = String(formData.get("prompt") ?? "");
    const trimmedPrompt = currentPrompt.trim();
    setPrompt(currentPrompt);
    if (!trimmedPrompt) {
      setError("请填写提示词");
      return;
    }
    if (promptLength(currentPrompt) > promptMaxLength) {
      setError(`提示词不能超过 ${promptMaxLength} 个字符`);
      return;
    }
    if (generationMode === "image") {
      if (!referenceImage) {
        setError("请上传参考图");
        return;
      }
      if (!acceptedReferenceImageTypes.has(referenceImage.type)) {
        setError("请上传 PNG 或 JPEG 图片");
        return;
      }
      if (referenceImage.size > maxReferenceImageBytes) {
        setError("参考图不能超过 5MB");
        return;
      }
    }

    setError("");
    setSubmitting(true);

    try {
      const task = await generationApi.createGeneration({
        prompt: trimmedPrompt,
        ratio,
        referenceImage: generationMode === "image" ? referenceImage ?? undefined : undefined,
      });
      setCurrentTask(task);
      refreshUserCredits();
    } catch (err) {
      setError(err instanceof Error ? err.message : "提交生成失败");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleCancel() {
    if (!currentTask || currentTask.status !== "queued") return;

    setError("");
    setCanceling(true);
    try {
      const body = await api<unknown>(`/api/generations/${currentTask.id}/cancel`, {
        method: "POST",
      });
      setCurrentTask(normalizeGenerationTask(body as Parameters<typeof normalizeGenerationTask>[0]));
      refreshUserCredits();
    } catch (err) {
      setError(err instanceof Error ? err.message : "取消生成失败");
    } finally {
      setCanceling(false);
    }
  }

  const disabled = submitting || canceling || isActiveTask(currentTask);
  const failureDetail = currentTask ? safeFailureDetail(currentTask) : "";
  const currentPromptLength = promptLength(prompt);
  const isPromptTooLong = currentPromptLength > promptMaxLength;
  const generationCreditCost = generationMode === "image" ? 2 : 1;

  return (
    <section className="workspace-page animate-fade-in" aria-labelledby="workspace-title">
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
        <form className="generator-form panel glass-panel" onSubmit={handleSubmit}>
          <div className="balance-card">
            <div className="balance-row">
              <span>当前余额</span>
              <strong>{user.creditBalance} 点</strong>
            </div>
            <div className="balance-row" style={{ fontSize: '12px', opacity: 0.8 }}>
              <span>今日免费: {user.dailyFreeCreditBalance}/{user.dailyFreeCreditLimit}</span>
              <span>付费额度: {user.paidCreditBalance}</span>
            </div>
          </div>
          
          <p className="usage-note">
            输入提示词，选择画面比例后开始生成。文生图消耗 1 点，图生图消耗 2 点。
          </p>

          <div className="mode-switch" role="group" aria-label="生成模式">
            <button
              aria-pressed={generationMode === "text"}
              className={generationMode === "text" ? "segment active" : "segment"}
              onClick={() => selectGenerationMode("text")}
              type="button"
            >
              文生图
            </button>
            <button
              aria-pressed={generationMode === "image"}
              className={generationMode === "image" ? "segment active" : "segment"}
              onClick={() => selectGenerationMode("image")}
              type="button"
            >
              图生图
            </button>
          </div>

          {generationMode === "image" ? (
            <div className="reference-upload-panel">
              <label className="field-label-row" htmlFor="reference-image">
                <span>参考图</span>
                <span className="field-counter" style={{ fontSize: '11px' }}>PNG/JPEG · ≤5MB</span>
              </label>
              <input
                accept="image/png,image/jpeg"
                aria-label="参考图"
                disabled={disabled}
                id="reference-image"
                onChange={handleReferenceImageChange}
                type="file"
              />
              {referencePreviewURL ? (
                <div className="reference-preview-wrap">
                  <img alt="参考图预览" className="reference-preview" src={referencePreviewURL} />
                  <button className="secondary-button" onClick={clearReferenceImage} type="button" style={{ width: '100%', minHeight: '36px' }}>
                    移除参考图
                  </button>
                </div>
              ) : (
                <p className="field-help" style={{ fontSize: '12px', marginTop: '4px' }}>上传一张参考图，再用提示词描述想生成的新画面。</p>
              )}
            </div>
          ) : null}

          <label className="field">
            <span className="field-label-row">
              <span>提示词</span>
              <span className={isPromptTooLong ? "field-counter over-limit" : "field-counter"}>
                {currentPromptLength}/{promptMaxLength}
              </span>
            </span>
            <textarea
              aria-label="提示词"
              name="prompt"
              onChange={(event) => setPrompt(event.target.value)}
              placeholder="描述你想生成的画面..."
              required
              rows={5}
              value={prompt}
              disabled={disabled}
            />
          </label>

          <fieldset className="ratio-control">
            <legend style={{ fontSize: '14px', fontWeight: 600, marginBottom: '8px' }}>画面比例</legend>
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
            {submitting ? "提交中..." : `开始生成 (${generationCreditCost} 点)`}
          </button>

          <PrivateSupportCard />
        </form>

        <section className="current-task panel glass-panel animate-fade-in" aria-label="当前任务">
          <div className="task-header">
            <div>
              <p className="eyebrow">任务状态</p>
              <h3>{currentTask ? statusText(currentTask.status) : "等待开始"}</h3>
            </div>
            {currentTask ? <span className={`status-badge ${currentTask.status}`}>{statusText(currentTask.status)}</span> : null}
          </div>

          {currentTask ? (
            <div className="task-detail">
              <p className="task-prompt" style={{ fontStyle: 'italic', opacity: 0.9 }}>"{currentTask.prompt}"</p>
              <dl className="meta-list">
                <div className="panel" style={{ padding: '10px' }}>
                  <dt>比例</dt>
                  <dd>{currentTask.ratio}</dd>
                </div>
                <div className="panel" style={{ padding: '10px' }}>
                  <dt>尺寸</dt>
                  <dd>{currentTask.size}</dd>
                </div>
              </dl>

              {currentTask.status !== "canceled" ? <GenerationProgress task={currentTask} now={progressNow} /> : null}
              
              <div style={{ display: 'flex', gap: '12px', marginTop: '12px' }}>
                {currentTask.status === "queued" ? (
                  <button
                    className="secondary-button cancel-generation-button"
                    disabled={canceling}
                    onClick={handleCancel}
                    type="button"
                  >
                    {canceling ? "取消中..." : "取消本次提交"}
                  </button>
                ) : null}
              </div>

              {currentTask.status === "canceled" ? (
                <p className="muted-text">已取消，本次额度已退回。</p>
              ) : null}
              
              {currentTask.status === "failed" ? (
                <div className="form-error" style={{ display: 'grid', gap: '4px' }}>
                  <span>生成失败，点数已退回。</span>
                  {failureDetail ? <span style={{ fontSize: '12px', opacity: 0.8 }}>原因: {failureDetail}</span> : null}
                </div>
              ) : null}

              {currentTask.status === "succeeded" && currentTask.imageUrl ? (
                <div className="animate-fade-in" style={{ display: 'grid', gap: '16px' }}>
                  <button
                    aria-label={`预览图片：${currentTask.prompt}`}
                    className="image-preview-trigger"
                    onClick={() => setPreviewImage({ alt: currentTask.prompt, src: currentTask.imageUrl! })}
                    type="button"
                  >
                    <img className="result-preview" src={currentTask.imageUrl} alt={currentTask.prompt} />
                  </button>
                  <a
                    className="primary-button download-button"
                    download={generationImageFilename(currentTask)}
                    href={currentTask.imageUrl}
                  >
                    下载高清原图
                  </a>
                </div>
              ) : null}
            </div>
          ) : (
            <div className="empty-state">
               <p>在左侧填写提示词并点击生成</p>
               <p style={{ fontSize: '13px', opacity: 0.7 }}>您的灵感将在此处绽放</p>
            </div>
          )}
        </section>
      </div>
      {previewImage ? (
        <ImagePreviewDialog
          alt={previewImage.alt}
          onClose={() => setPreviewImage(null)}
          src={previewImage.src}
        />
      ) : null}
    </section>
  );
}
