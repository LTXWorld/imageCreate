import { Fragment, FormEvent, useEffect, useState } from "react";

import {
  api,
  normalizeAdminAuditLogs,
  normalizeAdminGenerationTasks,
  normalizeAdminInvites,
  normalizeAdminUsers,
  type AdminAuditLog,
  type AdminGenerationTask,
  type AdminInvite,
  type AdminUser,
  type User,
} from "../api/client";

type AdminPageProps = {
  user: User;
};

type AdminTab = "users" | "invites" | "credits" | "security" | "audit";
type GenerationStatusFilter = AdminGenerationTask["status"] | "all";

type CreditDraft = {
  amount: string;
  reason: string;
  mode: "increase" | "decrease";
};

const tabs: Array<{ id: AdminTab; label: string }> = [
  { id: "users", label: "用户" },
  { id: "invites", label: "邀请码" },
  { id: "credits", label: "额度" },
  { id: "security", label: "安全" },
  { id: "audit", label: "审计" },
];

const generationStatusFilterOptions: Array<{ value: GenerationStatusFilter; label: string }> = [
  { value: "all", label: "全部状态" },
  { value: "succeeded", label: "成功" },
  { value: "failed", label: "失败" },
  { value: "queued", label: "排队中" },
  { value: "running", label: "生成中" },
  { value: "canceled", label: "已取消" },
];

function formatTime(value: string | undefined) {
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

function metadataText(value: unknown) {
  if (!value) return "-";
  if (typeof value === "string") return value;
  return JSON.stringify(value);
}

function taskStatusLabel(status: AdminGenerationTask["status"]) {
  const labels: Record<AdminGenerationTask["status"], string> = {
    queued: "排队中",
    running: "生成中",
    succeeded: "成功",
    failed: "失败",
    canceled: "已取消",
  };
  return labels[status] ?? status;
}

function taskFailureReason(task: AdminGenerationTask) {
  if (task.status !== "failed") return "-";
  return task.errorMessage || task.errorCode || "-";
}

function formatLatency(ms: number) {
  return ms > 0 ? `${ms} ms` : "-";
}

function summarizeGenerationTasks(tasks: AdminGenerationTask[]) {
  const completedWithLatency = tasks.filter((task) => task.completedAt && task.latencyMs > 0);
  const latencyTotal = completedWithLatency.reduce((total, task) => total + task.latencyMs, 0);
  const succeeded = tasks.filter((task) => task.status === "succeeded").length;
  const failed = tasks.filter((task) => task.status === "failed").length;
  const canceled = tasks.filter((task) => task.status === "canceled").length;
  const active = tasks.filter((task) => task.status === "queued" || task.status === "running").length;
  const terminal = succeeded + failed + canceled;

  return {
    total: tasks.length,
    succeeded,
    failed,
    active,
    successRate: terminal > 0 ? Math.round((succeeded / terminal) * 100) : 0,
    averageLatencyMs: completedWithLatency.length > 0 ? Math.round(latencyTotal / completedWithLatency.length) : 0,
  };
}

export function AdminPage({ user }: AdminPageProps) {
  const [activeTab, setActiveTab] = useState<AdminTab>("users");
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [invites, setInvites] = useState<AdminInvite[]>([]);
  const [auditLogs, setAuditLogs] = useState<AdminAuditLog[]>([]);
  const [generationTasks, setGenerationTasks] = useState<AdminGenerationTask[]>([]);
  const [generationUserFilter, setGenerationUserFilter] = useState("all");
  const [generationStatusFilter, setGenerationStatusFilter] = useState<GenerationStatusFilter>("all");
  const [inviteCode, setInviteCode] = useState("");
  const [inviteCredits, setInviteCredits] = useState("5");
  const [creditDrafts, setCreditDrafts] = useState<Record<string, CreditDraft>>({});
  const [ownPasswordDraft, setOwnPasswordDraft] = useState({
    currentPassword: "",
    newPassword: "",
    confirmPassword: "",
  });
  const [resetPasswordUserId, setResetPasswordUserId] = useState("");
  const [resetPasswordDraft, setResetPasswordDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const filteredGenerationTasks = generationTasks.filter((task) => {
    const matchesUser = generationUserFilter === "all" || task.userId === generationUserFilter;
    const matchesStatus = generationStatusFilter === "all" || task.status === generationStatusFilter;
    return matchesUser && matchesStatus;
  });
  const generationSummary = summarizeGenerationTasks(filteredGenerationTasks);

  useEffect(() => {
    if (user.role !== "admin") {
      setLoading(false);
      return;
    }

    let active = true;
    setError("");
    setLoading(true);

    Promise.all([
      api<unknown>("/api/admin/users"),
      api<unknown>("/api/admin/invites"),
      api<unknown>("/api/admin/audit-logs"),
      api<unknown>("/api/admin/generation-tasks"),
    ])
      .then(([usersBody, invitesBody, auditBody, tasksBody]) => {
        if (!active) return;
        setUsers(normalizeAdminUsers(usersBody));
        setInvites(normalizeAdminInvites(invitesBody));
        setAuditLogs(normalizeAdminAuditLogs(auditBody));
        setGenerationTasks(normalizeAdminGenerationTasks(tasksBody));
      })
      .catch((err) => {
        if (!active) return;
        setError(err instanceof Error ? err.message : "加载后台数据失败");
      })
      .finally(() => {
        if (!active) return;
        setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [user.role]);

  if (user.role !== "admin") {
    return (
      <section className="admin-page" aria-labelledby="admin-title">
        <div className="section-heading">
          <p className="eyebrow">管理后台</p>
          <h2 id="admin-title">无权访问</h2>
        </div>
        <div className="panel history-empty">当前账号不是管理员。</div>
      </section>
    );
  }

  async function handleCreateInvite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy("invite");
    setError("");
    setNotice("");

    try {
      const body = await api<{ invite: unknown }>("/api/admin/invites", {
        method: "POST",
        body: JSON.stringify({
          code: inviteCode.trim(),
          initial_credits: Number(inviteCredits),
        }),
      });
      const [invite] = normalizeAdminInvites({ invites: [body.invite] });
      if (invite) {
        setInvites((current) => [invite, ...current]);
      }
      setInviteCode("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "创建邀请码失败");
    } finally {
      setBusy("");
    }
  }

  async function handleStatusChange(target: AdminUser, status: AdminUser["status"]) {
    setBusy(`status-${target.id}`);
    setError("");
    setNotice("");

    try {
      const body = await api<{ user: unknown }>(`/api/admin/users/${target.id}/status`, {
        method: "PATCH",
        body: JSON.stringify({ status }),
      });
      const [updated] = normalizeAdminUsers({ users: [body.user] });
      if (updated) {
        setUsers((current) => current.map((item) => item.id === updated.id ? updated : item));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "更新用户状态失败");
    } finally {
      setBusy("");
    }
  }

  async function handleCreditSubmit(event: FormEvent<HTMLFormElement>, target: AdminUser) {
    event.preventDefault();
    const draft = creditDrafts[target.id] ?? { amount: "", reason: "", mode: "increase" };
    const numericAmount = Number(draft.amount);
    const signedAmount = draft.mode === "decrease" ? -numericAmount : numericAmount;

    setBusy(`credits-${target.id}`);
    setError("");
    setNotice("");

    try {
      const body = await api<{ user: unknown }>(`/api/admin/users/${target.id}/credits`, {
        method: "POST",
        body: JSON.stringify({
          amount: signedAmount,
          reason: draft.reason.trim(),
        }),
      });
      const [updated] = normalizeAdminUsers({ users: [body.user] });
      if (updated) {
        setUsers((current) => current.map((item) => item.id === updated.id ? updated : item));
      }
      setCreditDrafts((current) => ({
        ...current,
        [target.id]: { amount: "", reason: "", mode: "increase" },
      }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "调整额度失败");
    } finally {
      setBusy("");
    }
  }

  function updateCreditDraft(userID: string, patch: Partial<CreditDraft>) {
    setCreditDrafts((current) => ({
      ...current,
      [userID]: {
        amount: current[userID]?.amount ?? "",
        reason: current[userID]?.reason ?? "",
        mode: current[userID]?.mode ?? "increase",
        ...patch,
      },
    }));
  }

  function handleTabChange(tab: AdminTab) {
    setActiveTab(tab);
    setError("");
    setNotice("");
  }

  async function handleOwnPasswordSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy("own-password");
    setError("");
    setNotice("");

    if (ownPasswordDraft.newPassword !== ownPasswordDraft.confirmPassword) {
      setError("两次输入的新密码不一致");
      setBusy("");
      return;
    }

    try {
      await api("/api/admin/password", {
        method: "POST",
        body: JSON.stringify({
          current_password: ownPasswordDraft.currentPassword,
          new_password: ownPasswordDraft.newPassword,
        }),
      });
      setOwnPasswordDraft({
        currentPassword: "",
        newPassword: "",
        confirmPassword: "",
      });
      setNotice("密码已更新");
    } catch (err) {
      setError(err instanceof Error ? err.message : "更新密码失败");
    } finally {
      setBusy("");
    }
  }

  async function handleResetPasswordSubmit(event: FormEvent<HTMLFormElement>, target: AdminUser) {
    event.preventDefault();
    setBusy(`reset-password-${target.id}`);
    setError("");
    setNotice("");

    try {
      await api(`/api/admin/users/${target.id}/password`, {
        method: "POST",
        body: JSON.stringify({ new_password: resetPasswordDraft }),
      });
      setResetPasswordUserId("");
      setResetPasswordDraft("");
      setNotice("用户密码已重置");
    } catch (err) {
      setError(err instanceof Error ? err.message : "重置用户密码失败");
    } finally {
      setBusy("");
    }
  }

  return (
    <section className="admin-page" aria-labelledby="admin-title">
      <div className="section-toolbar">
        <div className="section-heading">
          <p className="eyebrow">管理后台</p>
          <h2 id="admin-title">管理员控制台</h2>
        </div>
      </div>

      <div className="admin-tabs" role="tablist" aria-label="后台模块">
        {tabs.map((tab) => (
          <button
            aria-selected={activeTab === tab.id}
            className={activeTab === tab.id ? "admin-tab active" : "admin-tab"}
            key={tab.id}
            onClick={() => handleTabChange(tab.id)}
            role="tab"
            type="button"
          >
            {tab.label}
          </button>
        ))}
      </div>

      {error ? <p className="form-error" role="alert">{error}</p> : null}
      {notice ? <p className="form-success" role="status">{notice}</p> : null}
      {loading ? <div className="panel history-empty">正在加载后台数据...</div> : null}

      {!loading && activeTab === "users" ? (
        <section className="admin-section panel" aria-labelledby="users-title">
          <h3 id="users-title">用户管理</h3>
          <div className="table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>用户名</th>
                  <th>角色</th>
                  <th>状态</th>
                  <th>今日免费</th>
                  <th>付费额度</th>
                  <th>合计</th>
                  <th>注册时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {users.map((item) => (
                  <Fragment key={item.id}>
                    <tr>
                      <td>{item.username}</td>
                      <td>{item.role}</td>
                      <td>{item.status}</td>
                      <td>{item.dailyFreeCreditBalance}/{item.dailyFreeCreditLimit}</td>
                      <td>{item.paidCreditBalance}</td>
                      <td>{item.creditBalance}</td>
                      <td>{formatTime(item.createdAt)}</td>
                      <td>
                        <button
                          className="secondary-button compact-button"
                          disabled={busy === `status-${item.id}`}
                          onClick={() => void handleStatusChange(item, item.status === "active" ? "disabled" : "active")}
                          type="button"
                        >
                          {item.status === "active" ? "禁用" : "启用"}
                        </button>
                        <button
                          className="secondary-button compact-button"
                          onClick={() => {
                            setResetPasswordUserId(item.id);
                            setResetPasswordDraft("");
                            setError("");
                            setNotice("");
                          }}
                          type="button"
                        >
                          重置密码
                        </button>
                      </td>
                    </tr>
                    {resetPasswordUserId === item.id ? (
                      <tr>
                        <td colSpan={8}>
                          <form
                            className="inline-admin-form"
                            onSubmit={(event) => void handleResetPasswordSubmit(event, item)}
                          >
                            <label className="field">
                              <span>{item.username} 的新密码</span>
                              <input
                                aria-label={`${item.username} 的新密码`}
                                autoComplete="new-password"
                                minLength={6}
                                name="reset-password"
                                onChange={(event) => setResetPasswordDraft(event.target.value)}
                                required
                                type="password"
                                value={resetPasswordDraft}
                              />
                            </label>
                            <button
                              className="primary-button compact-button"
                              disabled={busy === `reset-password-${item.id}`}
                              type="submit"
                            >
                              确认重置
                            </button>
                            <button
                              className="secondary-button compact-button"
                              onClick={() => {
                                setResetPasswordUserId("");
                                setResetPasswordDraft("");
                              }}
                              type="button"
                            >
                              取消
                            </button>
                          </form>
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ) : null}

      {!loading && activeTab === "invites" ? (
        <section className="admin-grid">
          <form className="admin-section panel compact-form" onSubmit={handleCreateInvite}>
            <h3>创建邀请码</h3>
            <label className="field">
              <span>邀请码</span>
              <input
                onChange={(event) => setInviteCode(event.target.value)}
                placeholder="留空自动生成"
                value={inviteCode}
              />
            </label>
            <label className="field">
              <span>初始额度</span>
              <input
                min="0"
                onChange={(event) => setInviteCredits(event.target.value)}
                required
                type="number"
                value={inviteCredits}
              />
            </label>
            <button className="primary-button" disabled={busy === "invite"} type="submit">
              创建邀请码
            </button>
          </form>

          <section className="admin-section panel" aria-label="邀请码列表">
            <h3>邀请码列表</h3>
            <div className="table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>邀请码</th>
                    <th>初始额度</th>
                    <th>状态</th>
                    <th>创建时间</th>
                  </tr>
                </thead>
                <tbody>
                  {invites.map((invite) => (
                    <tr key={invite.id}>
                      <td>{invite.code}</td>
                      <td>{invite.initialCredits} 点</td>
                      <td>{invite.status}</td>
                      <td>{formatTime(invite.createdAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </section>
      ) : null}

      {!loading && activeTab === "credits" ? (
        <section className="admin-section panel" aria-labelledby="credits-title">
          <h3 id="credits-title">额度管理</h3>
          <p className="muted-text">点数规则：每次生成扣 1 点，失败自动退回 1 点。</p>
          <div className="table-wrap">
            <table className="admin-table credit-table">
              <thead>
                <tr>
                  <th>用户名</th>
                  <th>当前余额</th>
                  <th>模式</th>
                  <th>调整值</th>
                  <th>原因</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {users.map((item) => {
                  const draft = creditDrafts[item.id] ?? { amount: "", reason: "", mode: "increase" };
                  return (
                    <tr key={item.id}>
                      <td>{item.username}</td>
                      <td>{item.creditBalance} 点</td>
                      <td>
                        <select
                          aria-label="调整模式"
                          className="table-input"
                          onChange={(event) => updateCreditDraft(item.id, { mode: event.target.value as CreditDraft["mode"] })}
                          value={draft.mode}
                        >
                          <option value="increase">增加</option>
                          <option value="decrease">扣减</option>
                        </select>
                      </td>
                      <td>
                        <input
                          aria-label={`调整 ${item.username} 的积分`}
                          className="table-input number-input"
                          min="1"
                          onChange={(event) => updateCreditDraft(item.id, { amount: event.target.value })}
                          type="number"
                          value={draft.amount}
                        />
                      </td>
                      <td>
                        <input
                          aria-label={`调整 ${item.username} 的原因`}
                          className="table-input"
                          onChange={(event) => updateCreditDraft(item.id, { reason: event.target.value })}
                          value={draft.reason}
                        />
                      </td>
                      <td>
                        <form onSubmit={(event) => void handleCreditSubmit(event, item)}>
                          <button
                            className="primary-button compact-button"
                            disabled={busy === `credits-${item.id}`}
                            type="submit"
                          >
                            提交调整
                          </button>
                        </form>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      ) : null}

      {!loading && activeTab === "security" ? (
        <section className="admin-section panel" aria-labelledby="security-title">
          <h3 id="security-title">账号安全</h3>
          <form className="compact-form" onSubmit={handleOwnPasswordSubmit}>
            <label className="field">
              <span>当前密码</span>
              <input
                autoComplete="current-password"
                name="current-password"
                onChange={(event) => setOwnPasswordDraft((current) => ({
                  ...current,
                  currentPassword: event.target.value,
                }))}
                required
                type="password"
                value={ownPasswordDraft.currentPassword}
              />
            </label>
            <label className="field">
              <span>新密码</span>
              <input
                autoComplete="new-password"
                minLength={6}
                name="new-password"
                onChange={(event) => setOwnPasswordDraft((current) => ({
                  ...current,
                  newPassword: event.target.value,
                }))}
                required
                type="password"
                value={ownPasswordDraft.newPassword}
              />
            </label>
            <label className="field">
              <span>确认新密码</span>
              <input
                autoComplete="new-password"
                minLength={6}
                name="confirm-password"
                onChange={(event) => setOwnPasswordDraft((current) => ({
                  ...current,
                  confirmPassword: event.target.value,
                }))}
                required
                type="password"
                value={ownPasswordDraft.confirmPassword}
              />
            </label>
            <button className="primary-button" disabled={busy === "own-password"} type="submit">
              更新密码
            </button>
          </form>
        </section>
      ) : null}

      {!loading && activeTab === "audit" ? (
        <section className="admin-grid">
          <section className="admin-section panel" aria-label="任务审计">
            <h3 id="task-audit-title">任务审计</h3>
            <div className="admin-metrics" aria-label="生图结果汇总">
              <div className="admin-metric">
                <span>总任务</span>
                <strong>{generationSummary.total}</strong>
              </div>
              <div className="admin-metric">
                <span>成功数</span>
                <strong>{generationSummary.succeeded}</strong>
              </div>
              <div className="admin-metric">
                <span>失败数</span>
                <strong>{generationSummary.failed}</strong>
              </div>
              <div className="admin-metric">
                <span>进行中</span>
                <strong>{generationSummary.active}</strong>
              </div>
              <div className="admin-metric">
                <span>成功率</span>
                <strong>{generationSummary.successRate}%</strong>
              </div>
              <div className="admin-metric">
                <span>平均耗时</span>
                <strong>{formatLatency(generationSummary.averageLatencyMs)}</strong>
              </div>
            </div>
            <div className="admin-filters" aria-label="任务筛选">
              <label className="field compact-field">
                <span>用户</span>
                <select
                  aria-label="筛选用户"
                  onChange={(event) => setGenerationUserFilter(event.target.value)}
                  value={generationUserFilter}
                >
                  <option value="all">全部用户</option>
                  {users.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.username}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field compact-field">
                <span>状态</span>
                <select
                  aria-label="筛选状态"
                  onChange={(event) => setGenerationStatusFilter(event.target.value as GenerationStatusFilter)}
                  value={generationStatusFilter}
                >
                  {generationStatusFilterOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <div className="table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>用户</th>
                    <th>提示词</th>
                    <th>状态</th>
                    <th>失败原因</th>
                    <th>尺寸</th>
                    <th>耗时</th>
                    <th>时间</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredGenerationTasks.map((task) => (
                    <tr key={task.id}>
                      <td>{task.username}</td>
                      <td>{task.prompt}</td>
                      <td>{taskStatusLabel(task.status)}</td>
                      <td>{taskFailureReason(task)}</td>
                      <td>{task.size}</td>
                      <td>{formatLatency(task.latencyMs)}</td>
                      <td>{formatTime(task.createdAt)}</td>
                    </tr>
                  ))}
                  {filteredGenerationTasks.length === 0 ? (
                    <tr>
                      <td colSpan={7}>暂无匹配任务</td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </section>

          <section className="admin-section panel" aria-labelledby="audit-log-title">
            <h3 id="audit-log-title">操作记录</h3>
            <div className="table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>动作</th>
                    <th>目标用户</th>
                    <th>详情</th>
                    <th>时间</th>
                  </tr>
                </thead>
                <tbody>
                  {auditLogs.map((log) => (
                    <tr key={log.id}>
                      <td>{log.action}</td>
                      <td>{log.targetUserId ?? "-"}</td>
                      <td>{metadataText(log.metadata)}</td>
                      <td>{formatTime(log.createdAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </section>
      ) : null}
    </section>
  );
}
