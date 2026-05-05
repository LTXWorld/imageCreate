import { useEffect, useState } from "react";
import { api, generationImageFilename, normalizeGenerationList, type GenerationTask } from "../api/client";
import { ImagePreviewDialog } from "../components/ImagePreviewDialog";
import "../styles/History.css";
import "../styles/Components.css";

type HistoryPageProps = {
  onWorkspaceClick?: () => void;
};

type PreviewImage = {
  alt: string;
  src: string;
};

function statusText(status: GenerationTask["status"]) {
  if (status === "queued" || status === "running") return "生成中";
  if (status === "succeeded") return "已完成";
  if (status === "failed") return "生成失败";
  return "已取消";
}

function formatTime(value: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function HistoryPage({ onWorkspaceClick }: HistoryPageProps) {
  const [tasks, setTasks] = useState<GenerationTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deletingId, setDeletingId] = useState("");
  const [previewImage, setPreviewImage] = useState<PreviewImage | null>(null);

  async function loadHistory() {
    setError("");
    setLoading(true);
    try {
      const body = await api<unknown>("/api/generations");
      setTasks(normalizeGenerationList(body));
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载历史失败");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadHistory();
  }, []);

  async function handleDelete(id: string) {
    setDeletingId(id);
    setError("");
    try {
      await api<{ ok: boolean }>(`/api/generations/${id}`, { method: "DELETE" });
      await loadHistory();
    } catch (err) {
      setError(err instanceof Error ? err.message : "删除失败");
    } finally {
      setDeletingId("");
    }
  }

  return (
    <section className="history-page animate-fade-in" aria-labelledby="history-title">
      <div className="section-toolbar">
        <div className="section-heading">
          <p className="eyebrow">30 天记录</p>
          <h2 id="history-title">历史记录</h2>
          <p className="muted-text">记录将保留 30 天，请及时下载需要长期保存的图片。</p>
        </div>
        <button className="secondary-button" type="button" onClick={onWorkspaceClick}>
          返回创作台
        </button>
      </div>

      {error ? <p className="form-error" role="alert" style={{ marginBottom: '20px' }}>{error}</p> : null}
      
      {loading ? (
        <div className="empty-state">
           <p>正在加载历史记录...</p>
        </div>
      ) : tasks.length === 0 ? (
        <div className="empty-state">
           <p>暂无生成记录</p>
           <button className="primary-button" onClick={onWorkspaceClick} style={{ marginTop: '16px' }}>
              去生成一张
           </button>
        </div>
      ) : (
        <div className="history-list">
          {tasks.map((task) => (
            <article className="history-item panel glass-panel" key={task.id}>
              {task.status === "succeeded" && task.imageUrl ? (
                <button
                  aria-label={`预览图片：${task.prompt}`}
                  className="history-preview-trigger"
                  onClick={() => setPreviewImage({ alt: task.prompt, src: task.imageUrl! })}
                  type="button"
                >
                  <img className="history-preview" src={task.imageUrl} alt={task.prompt} />
                </button>
              ) : (
                <div className="history-preview-trigger" style={{ display: 'grid', placeItems: 'center', background: '#f8faf8', borderRadius: '12px' }}>
                   <span className={`status-badge ${task.status}`}>{statusText(task.status)}</span>
                </div>
              )}

              <div className="history-main">
                <div className="history-time">{formatTime(task.createdAt)}</div>
                <p className="history-prompt">{task.prompt}</p>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                  <span className="status-badge" style={{ background: 'var(--color-jade-light)', color: 'var(--color-jade-deep)', border: 'none' }}>{task.ratio}</span>
                  <span className="status-badge" style={{ background: 'var(--color-jade-light)', color: 'var(--color-jade-deep)', border: 'none' }}>{task.size}</span>
                </div>
                {task.status === "failed" && task.message ? (
                  <p className="form-error" style={{ padding: '4px 8px', fontSize: '12px' }}>{task.message}</p>
                ) : null}
              </div>

              <div className="history-actions">
                {task.status === "succeeded" && task.imageUrl ? (
                  <a
                    className="icon-button"
                    download={generationImageFilename(task)}
                    href={task.imageUrl}
                    title="下载图片"
                  >
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                  </a>
                ) : null}
                <button
                  className="icon-button"
                  disabled={deletingId === task.id}
                  onClick={() => void handleDelete(task.id)}
                  type="button"
                  title="删除记录"
                  style={{ color: '#c62828' }}
                >
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/></svg>
                </button>
              </div>
            </article>
          ))}
        </div>
      )}

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
