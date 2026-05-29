import { useEffect } from "react";
import { generationImageFilename, type GenerationTask } from "../api/client";

function formatDateTime(value: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function statusText(status: GenerationTask["status"]) {
  if (status === "queued" || status === "running") return "生成中";
  if (status === "succeeded") return "已完成";
  if (status === "failed") return "生成失败";
  return "已取消";
}

type ArtworkDetailDialogProps = {
  task: GenerationTask;
  onClose: () => void;
  onFavorite: (task: GenerationTask) => void;
  onReuseImage?: (task: GenerationTask) => void;
  onReusePrompt?: (task: GenerationTask) => void;
  updating?: boolean;
};

export function ArtworkDetailDialog({
  task,
  onClose,
  onFavorite,
  onReuseImage,
  onReusePrompt,
  updating = false,
}: ArtworkDetailDialogProps) {
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const canUseImage = task.status === "succeeded" && Boolean(task.imageUrl);

  return (
    <div
      aria-label="作品详情"
      aria-modal="true"
      className="image-preview-overlay"
      onClick={onClose}
      role="dialog"
    >
      <div className="artwork-detail-dialog" onClick={(event) => event.stopPropagation()}>
        <button
          aria-label="关闭作品详情"
          className="image-preview-close"
          onClick={onClose}
          type="button"
        >
          ×
        </button>

        <div className="artwork-detail-media">
          {canUseImage ? (
            <img className="artwork-detail-image" src={task.imageUrl} alt={task.prompt} />
          ) : (
            <div className="artwork-detail-placeholder">
              <span className={`status-badge ${task.status}`}>{statusText(task.status)}</span>
            </div>
          )}
        </div>

        <aside className="artwork-detail-panel">
          <div>
            <p className="eyebrow">作品详情</p>
            <h3>{task.title || "未命名作品"}</h3>
          </div>

          <div className="artwork-detail-actions">
            <button
              className="secondary-button"
              disabled={updating}
              onClick={() => onFavorite(task)}
              type="button"
            >
              {task.isFavorite ? "取消收藏" : "收藏作品"}
            </button>
            <button className="secondary-button" onClick={() => onReusePrompt?.(task)} type="button">
              复制提示词再生成
            </button>
            {canUseImage ? (
              <button className="secondary-button" onClick={() => onReuseImage?.(task)} type="button">
                作为参考图再创作
              </button>
            ) : null}
            {canUseImage ? (
              <a className="primary-button" download={generationImageFilename(task)} href={task.imageUrl}>
                下载图片
              </a>
            ) : null}
          </div>

          <dl className="artwork-detail-meta">
            <div>
              <dt>状态</dt>
              <dd><span className={`status-badge ${task.status}`}>{statusText(task.status)}</span></dd>
            </div>
            <div>
              <dt>比例</dt>
              <dd>{task.ratio}</dd>
            </div>
            <div>
              <dt>尺寸</dt>
              <dd>{task.size}</dd>
            </div>
            <div>
              <dt>创建时间</dt>
              <dd>{formatDateTime(task.createdAt)}</dd>
            </div>
          </dl>

          <section className="artwork-detail-prompt">
            <h4>提示词</h4>
            <p>{task.prompt}</p>
          </section>

          {task.status === "failed" && task.message ? (
            <p className="form-error">{task.message}</p>
          ) : null}
        </aside>
      </div>
    </div>
  );
}
