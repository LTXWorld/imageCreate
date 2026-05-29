import { useEffect, useState } from "react";
import { api, generationImageFilename, normalizeGenerationList, normalizeGenerationTask, type GenerationTask } from "../api/client";
import { ArtworkDetailDialog } from "../components/ArtworkDetailDialog";
import "../styles/History.css";
import "../styles/Components.css";

type HistoryPageProps = {
  onReuseImage?: (task: GenerationTask) => void;
  onReusePrompt?: (task: GenerationTask) => void;
  onWorkspaceClick?: () => void;
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

export function HistoryPage({ onReuseImage, onReusePrompt, onWorkspaceClick }: HistoryPageProps) {
  const [tasks, setTasks] = useState<GenerationTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deletingId, setDeletingId] = useState("");
  const [updatingId, setUpdatingId] = useState("");
  const [editingTitleId, setEditingTitleId] = useState("");
  const [titleDraft, setTitleDraft] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "favorite" | GenerationTask["status"]>("all");
  const [detailTaskId, setDetailTaskId] = useState("");

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

  function updateTaskInList(nextTask: GenerationTask) {
    setTasks((currentTasks) => currentTasks.map((task) => task.id === nextTask.id ? nextTask : task));
  }

  async function handleFavorite(task: GenerationTask) {
    setUpdatingId(task.id);
    setError("");
    try {
      const body = await api<unknown>(`/api/generations/${task.id}/favorite`, {
        method: "PATCH",
        body: JSON.stringify({ favorite: !task.isFavorite }),
      });
      updateTaskInList(normalizeGenerationTask(body as Parameters<typeof normalizeGenerationTask>[0]));
    } catch (err) {
      setError(err instanceof Error ? err.message : "更新收藏失败");
    } finally {
      setUpdatingId("");
    }
  }

  async function handleSaveTitle(task: GenerationTask) {
    setUpdatingId(task.id);
    setError("");
    try {
      const body = await api<unknown>(`/api/generations/${task.id}/title`, {
        method: "PATCH",
        body: JSON.stringify({ title: titleDraft }),
      });
      updateTaskInList(normalizeGenerationTask(body as Parameters<typeof normalizeGenerationTask>[0]));
      setEditingTitleId("");
      setTitleDraft("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存标题失败");
    } finally {
      setUpdatingId("");
    }
  }

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

  const filteredTasks = tasks.filter((task) => {
    if (statusFilter === "all") return true;
    if (statusFilter === "favorite") return task.isFavorite;
    return task.status === statusFilter;
  });
  const detailTask = tasks.find((task) => task.id === detailTaskId) ?? null;

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

      <div className="panel glass-panel" style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', marginBottom: '20px' }} role="group" aria-label="历史筛选">
        {[
          ["all", "全部"],
          ["favorite", "已收藏"],
          ["succeeded", "已完成"],
          ["failed", "失败"],
          ["canceled", "已取消"],
        ].map(([value, label]) => (
          <button
            aria-pressed={statusFilter === value}
            className={statusFilter === value ? "segment active" : "segment"}
            key={value}
            onClick={() => setStatusFilter(value as typeof statusFilter)}
            type="button"
          >
            {label}
          </button>
        ))}
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
      ) : filteredTasks.length === 0 ? (
        <div className="empty-state">
           <p>当前筛选下暂无作品</p>
        </div>
      ) : (
        <div className="history-list">
          {filteredTasks.map((task) => (
            <article className="history-item panel glass-panel" key={task.id}>
              {task.status === "succeeded" && task.imageUrl ? (
                <button
                  aria-label={`查看作品详情：${task.prompt}`}
                  className="history-preview-trigger"
                  onClick={() => setDetailTaskId(task.id)}
                  type="button"
                >
                  <img className="history-preview" src={task.imageUrl} alt={task.prompt} />
                </button>
              ) : (
                <button
                  aria-label={`查看作品详情：${task.prompt}`}
                  className="history-preview-trigger"
                  onClick={() => setDetailTaskId(task.id)}
                  style={{ display: 'grid', placeItems: 'center', background: '#f8faf8', borderRadius: '12px', border: 'none' }}
                  type="button"
                >
                   <span className={`status-badge ${task.status}`}>{statusText(task.status)}</span>
                </button>
              )}

              <div className="history-main">
                <div className="history-time">{formatTime(task.createdAt)}</div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                  <strong>{task.title || "未命名作品"}</strong>
                  {task.isFavorite ? <span aria-label="已收藏" title="已收藏">★</span> : null}
                </div>
                {editingTitleId === task.id ? (
                  <form
                    onSubmit={(event) => {
                      event.preventDefault();
                      void handleSaveTitle(task);
                    }}
                    style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}
                  >
                    <input
                      aria-label="作品标题"
                      maxLength={80}
                      onChange={(event) => setTitleDraft(event.target.value)}
                      placeholder="输入作品标题"
                      style={{ minHeight: '34px', flex: '1 1 180px' }}
                      value={titleDraft}
                    />
                    <button className="secondary-button" disabled={updatingId === task.id} type="submit">保存</button>
                    <button
                      className="secondary-button"
                      onClick={() => {
                        setEditingTitleId("");
                        setTitleDraft("");
                      }}
                      type="button"
                    >取消</button>
                  </form>
                ) : null}
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
                <button
                  className="icon-button"
                  disabled={updatingId === task.id}
                  onClick={() => void handleFavorite(task)}
                  type="button"
                  title={task.isFavorite ? "取消收藏" : "收藏作品"}
                  aria-label={task.isFavorite ? "取消收藏" : "收藏作品"}
                  style={{ color: task.isFavorite ? '#d18b00' : undefined }}
                >
                  <svg width="18" height="18" viewBox="0 0 24 24" fill={task.isFavorite ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
                </button>
                <button
                  className="icon-button"
                  onClick={() => {
                    setEditingTitleId(task.id);
                    setTitleDraft(task.title ?? "");
                  }}
                  type="button"
                  title="编辑标题"
                  aria-label="编辑标题"
                >
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/></svg>
                </button>
                <button
                  className="icon-button"
                  onClick={() => onReusePrompt?.(task)}
                  type="button"
                  title="复制提示词再生成"
                  aria-label="复制提示词再生成"
                >
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                </button>
                {task.status === "succeeded" && task.imageUrl ? (
                  <button
                    className="icon-button"
                    onClick={() => onReuseImage?.(task)}
                    type="button"
                    title="作为参考图再创作"
                    aria-label="作为参考图再创作"
                  >
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M3 21v-5h5"/><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M16 3h5v5"/></svg>
                  </button>
                ) : null}
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

      {detailTask ? (
        <ArtworkDetailDialog
          onClose={() => setDetailTaskId("")}
          onFavorite={(task) => void handleFavorite(task)}
          onReuseImage={onReuseImage}
          onReusePrompt={onReusePrompt}
          task={detailTask}
          updating={updatingId === detailTask.id}
        />
      ) : null}
    </section>
  );
}
