import { useEffect, useState } from "react";

import { api, normalizeGenerationList, type GenerationTask } from "../api/client";

type HistoryPageProps = {
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

export function HistoryPage({ onWorkspaceClick }: HistoryPageProps) {
  const [tasks, setTasks] = useState<GenerationTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deletingId, setDeletingId] = useState("");

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
    <section className="history-page" aria-labelledby="history-title">
      <div className="section-toolbar">
        <div className="section-heading">
          <p className="eyebrow">30 天记录</p>
          <h2 id="history-title">历史记录</h2>
        </div>
        <button className="secondary-button" type="button" onClick={onWorkspaceClick}>
          返回创作台
        </button>
      </div>

      {error ? <p className="form-error" role="alert">{error}</p> : null}
      {loading ? <div className="panel history-empty">正在加载...</div> : null}
      {!loading && tasks.length === 0 ? <div className="panel history-empty">暂无生成记录</div> : null}

      <div className="history-list">
        {tasks.map((task) => (
          <article className="history-item panel" key={task.id}>
            <div className="history-main">
              <div className="history-title-row">
                <span className={`status-badge ${task.status}`}>{statusText(task.status)}</span>
                <span className="history-time">{formatTime(task.createdAt)}</span>
              </div>
              <p className="task-prompt">{task.prompt}</p>
              <dl className="meta-list">
                <div>
                  <dt>比例</dt>
                  <dd>{task.ratio}</dd>
                </div>
                <div>
                  <dt>尺寸</dt>
                  <dd>{task.size}</dd>
                </div>
              </dl>
              {task.status === "failed" && task.message ? (
                <p className="history-message">{task.message}</p>
              ) : null}
            </div>

            {task.status === "succeeded" && task.imageUrl ? (
              <img className="history-preview" src={task.imageUrl} alt={task.prompt} />
            ) : null}

            <button
              className="secondary-button delete-button"
              disabled={deletingId === task.id}
              onClick={() => void handleDelete(task.id)}
              type="button"
            >
              {deletingId === task.id ? "删除中..." : "删除"}
            </button>
          </article>
        ))}
      </div>
    </section>
  );
}
