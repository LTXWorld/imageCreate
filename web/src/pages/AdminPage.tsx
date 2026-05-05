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
import "../styles/Admin.css";
import "../styles/Components.css";

type AdminPageProps = {
  user: User;
  onUserUpdate?: (user: User) => void;
};

type AdminTab = "users" | "invites" | "credits" | "security" | "audit";
type GenerationStatusFilter = AdminGenerationTask["status"] | "all";

type CreditDraft = {
  amount: string;
  reason: string;
  mode: "increase" | "decrease";
};

type DailyFreeLimitDraft = string;
type DailyFreeBalanceDraft = string;

const tabs: Array<{ id: AdminTab; label: string }> = [
  { id: "users", label: "用户管理" },
  { id: "invites", label: "邀请码" },
  { id: "credits", label: "额度管理" },
  { id: "audit", label: "审计" },
  { id: "security", label: "安全" },
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

export function AdminPage({ user, onUserUpdate }: AdminPageProps) {
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
  const [dailyFreeLimitDrafts, setDailyFreeLimitDrafts] = useState<Record<string, DailyFreeLimitDraft>>({});
  const [dailyFreeBalanceDrafts, setDailyFreeBalanceDrafts] = useState<Record<string, DailyFreeBalanceDraft>>({});
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

  function applyUpdatedUser(updated: AdminUser) {
    setUsers((current) => current.map((item) => item.id === updated.id ? updated : item));
    if (updated.id === user.id) {
      onUserUpdate?.(updated);
    }
  }

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
      <section className="admin-page animate-fade-in" aria-labelledby="admin-title">
        <div className="section-heading">
          <p className="eyebrow">管理后台</p>
          <h2 id="admin-title">无权访问</h2>
        </div>
        <div className="panel glass-panel empty-state">当前账号不是管理员。</div>
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
        applyUpdatedUser(updated);
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
        applyUpdatedUser(updated);
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

  async function handleDailyFreeLimitSubmit(event: FormEvent<HTMLFormElement>, target: AdminUser) {
    event.preventDefault();
    const draft = dailyFreeLimitDrafts[target.id] ?? "";

    setBusy(`daily-free-limit-${target.id}`);
    setError("");
    setNotice("");

    try {
      const body = await api<{ user: unknown }>(`/api/admin/users/${target.id}/daily-free-limit`, {
        method: "PATCH",
        body: JSON.stringify({
          daily_free_credit_limit: Number(draft),
        }),
      });
      const [updated] = normalizeAdminUsers({ users: [body.user] });
      if (updated) {
        applyUpdatedUser(updated);
      }
      setDailyFreeLimitDrafts((current) => ({
        ...current,
        [target.id]: "",
      }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "更新每日免费上限失败");
    } finally {
      setBusy("");
    }
  }

  async function handleDailyFreeBalanceSubmit(event: FormEvent<HTMLFormElement>, target: AdminUser) {
    event.preventDefault();
    const draft = dailyFreeBalanceDrafts[target.id] ?? "";

    setBusy(`daily-free-balance-${target.id}`);
    setError("");
    setNotice("");

    try {
      const body = await api<{ user: unknown }>(`/api/admin/users/${target.id}/daily-free-balance`, {
        method: "PATCH",
        body: JSON.stringify({
          amount: Number(draft),
        }),
      });
      const [updated] = normalizeAdminUsers({ users: [body.user] });
      if (updated) {
        applyUpdatedUser(updated);
      }
      setDailyFreeBalanceDrafts((current) => ({
        ...current,
        [target.id]: "",
      }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "补今日免费额度失败");
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

  function updateDailyFreeLimitDraft(userID: string, value: string) {
    setDailyFreeLimitDrafts((current) => ({
      ...current,
      [userID]: value,
    }));
  }

  function updateDailyFreeBalanceDraft(userID: string, value: string) {
    setDailyFreeBalanceDrafts((current) => ({
      ...current,
      [userID]: value,
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
    <section className="admin-page animate-fade-in" aria-labelledby="admin-title">
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

      {error ? <p className="form-error" role="alert" style={{ marginBottom: '20px' }}>{error}</p> : null}
      {notice ? <p className="status-badge succeeded" style={{ marginBottom: '20px', display: 'block', width: 'fit-content' }}>{notice}</p> : null}
      
      {loading ? (
         <div className="empty-state">正在加载后台数据...</div>
      ) : (
        <>
          {activeTab === "users" ? (
            <section className="admin-section panel glass-panel animate-fade-in" aria-labelledby="users-title">
              <h3 id="users-title">用户管理</h3>
              <div className="admin-table-wrap">
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
                          <td><strong>{item.username}</strong></td>
                          <td><span className="status-badge" style={{ background: '#eee' }}>{item.role}</span></td>
                          <td><span className={`status-badge ${item.status === 'active' ? 'succeeded' : 'failed'}`}>{item.status}</span></td>
                          <td>{item.dailyFreeCreditBalance}/{item.dailyFreeCreditLimit}</td>
                          <td>{item.paidCreditBalance}</td>
                          <td><strong>{item.creditBalance}</strong></td>
                          <td>{formatTime(item.createdAt)}</td>
                          <td>
                            <div style={{ display: 'flex', gap: '8px' }}>
                              <button
                                className="secondary-button"
                                style={{ minHeight: '32px', padding: '0 12px', fontSize: '13px' }}
                                disabled={busy === `status-${item.id}`}
                                onClick={() => void handleStatusChange(item, item.status === "active" ? "disabled" : "active")}
                                type="button"
                              >
                                {item.status === "active" ? "禁用" : "启用"}
                              </button>
                              <button
                                className="secondary-button"
                                style={{ minHeight: '32px', padding: '0 12px', fontSize: '13px' }}
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
                            </div>
                          </td>
                        </tr>
                        {resetPasswordUserId === item.id ? (
                          <tr>
                            <td colSpan={8} style={{ background: '#f8faf8' }}>
                              <form
                                style={{ display: 'flex', gap: '12px', alignItems: 'flex-end' }}
                                onSubmit={(event) => void handleResetPasswordSubmit(event, item)}
                              >
                                <label className="field" style={{ marginBottom: 0 }}>
                                  <span style={{ fontSize: '12px' }}>{item.username} 的新密码</span>
                                  <input
                                    aria-label={`${item.username} 的新密码`}
                                    autoComplete="new-password"
                                    minLength={6}
                                    name="reset-password"
                                    onChange={(event) => setResetPasswordDraft(event.target.value)}
                                    required
                                    type="password"
                                    value={resetPasswordDraft}
                                    style={{ minHeight: '36px' }}
                                  />
                                </label>
                                <button
                                  className="primary-button"
                                  style={{ minHeight: '36px' }}
                                  disabled={busy === `reset-password-${item.id}`}
                                  type="submit"
                                >
                                  确认重置
                                </button>
                                <button
                                  className="secondary-button"
                                  style={{ minHeight: '36px' }}
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

          {activeTab === "invites" ? (
            <div className="admin-grid">
              <form className="admin-section panel glass-panel compact-form animate-fade-in" onSubmit={handleCreateInvite}>
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

              <section className="admin-section panel glass-panel animate-fade-in" aria-label="邀请码列表">
                <h3>邀请码列表</h3>
                <div className="admin-table-wrap">
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
                          <td><strong>{invite.code}</strong></td>
                          <td>{invite.initialCredits} 点</td>
                          <td><span className="status-badge succeeded">{invite.status}</span></td>
                          <td>{formatTime(invite.createdAt)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </div>
          ) : null}

          {activeTab === "credits" ? (
            <section className="admin-section panel glass-panel animate-fade-in" aria-labelledby="credits-title">
              <h3 id="credits-title">额度管理</h3>
              <p className="usage-note" style={{ marginBottom: '20px' }}>文生图扣 1 点，图生图扣 2 点，失败自动退回。</p>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>用户名</th>
                      <th>余额</th>
                      <th>免费额度</th>
                      <th>每日上限</th>
                      <th>模式</th>
                      <th>值</th>
                      <th>原因</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {users.map((item) => {
                      const draft = creditDrafts[item.id] ?? { amount: "", reason: "", mode: "increase" };
                      const dailyFreeLimitDraft = dailyFreeLimitDrafts[item.id] ?? String(item.dailyFreeCreditLimit);
                      
                      return (
                        <tr key={item.id}>
                          <td><strong>{item.username}</strong></td>
                          <td><strong>{item.creditBalance}</strong></td>
                          <td>{item.dailyFreeCreditBalance}/{item.dailyFreeCreditLimit}</td>
                          <td>
                            <form onSubmit={(event) => void handleDailyFreeLimitSubmit(event, item)} style={{ display: 'flex', gap: '4px' }}>
                              <input
                                aria-label="每日免费上限"
                                style={{ width: '60px', minHeight: '32px', padding: '4px 8px' }}
                                min="0"
                                onChange={(event) => updateDailyFreeLimitDraft(item.id, event.target.value)}
                                type="number"
                                value={dailyFreeLimitDraft}
                              />
                              <button
                                className="secondary-button"
                                style={{ minHeight: '32px', padding: '0 8px', fontSize: '12px' }}
                                disabled={busy === `daily-free-limit-${item.id}`}
                                type="submit"
                              >
                                更新
                              </button>
                            </form>
                          </td>
                          <td>
                            <select
                              aria-label="调整模式"
                              style={{ minHeight: '32px', padding: '4px', borderRadius: '8px', border: '1px solid var(--color-border)' }}
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
                              style={{ width: '60px', minHeight: '32px', padding: '4px 8px', borderRadius: '8px', border: '1px solid var(--color-border)' }}
                              min="1"
                              onChange={(event) => updateCreditDraft(item.id, { amount: event.target.value })}
                              type="number"
                              value={draft.amount}
                            />
                          </td>
                          <td>
                            <input
                              aria-label={`调整 ${item.username} 的原因`}
                              style={{ width: '120px', minHeight: '32px', padding: '4px 8px', borderRadius: '8px', border: '1px solid var(--color-border)' }}
                              onChange={(event) => updateCreditDraft(item.id, { reason: event.target.value })}
                              type="text"
                              value={draft.reason}
                            />
                          </td>
                          <td>
                            <form onSubmit={(event) => void handleCreditSubmit(event, item)}>
                              <button
                                className="primary-button"
                                style={{ minHeight: '32px', padding: '0 12px', fontSize: '13px' }}
                                disabled={busy === `credits-${item.id}`}
                                type="submit"
                              >
                                提交
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

          {activeTab === "audit" ? (
            <div style={{ display: 'grid', gap: '32px' }}>
              <section className="admin-section panel glass-panel animate-fade-in" aria-label="任务审计">
                <h3>任务审计</h3>
                <div className="admin-metrics">
                  <div className="admin-metric panel" style={{ background: '#fafffd' }}>
                    <span>总任务</span>
                    <strong>{generationSummary.total}</strong>
                  </div>
                  <div className="admin-metric panel" style={{ background: '#fafffd' }}>
                    <span>成功数</span>
                    <strong>{generationSummary.succeeded}</strong>
                  </div>
                  <div className="admin-metric panel" style={{ background: '#fafffd' }}>
                    <span>成功率</span>
                    <strong>{generationSummary.successRate}%</strong>
                  </div>
                  <div className="admin-metric panel" style={{ background: '#fafffd' }}>
                    <span>平均耗时</span>
                    <strong>{formatLatency(generationSummary.averageLatencyMs)}</strong>
                  </div>
                </div>
                
                <div className="admin-filters">
                  <label className="field" style={{ marginBottom: 0, minWidth: '160px' }}>
                    <span>用户筛选</span>
                    <select
                      onChange={(event) => setGenerationUserFilter(event.target.value)}
                      value={generationUserFilter}
                    >
                      <option value="all">全部用户</option>
                      {users.map((item) => (
                        <option key={item.id} value={item.id}>{item.username}</option>
                      ))}
                    </select>
                  </label>
                  <label className="field" style={{ marginBottom: 0, minWidth: '160px' }}>
                    <span>状态筛选</span>
                    <select
                      onChange={(event) => setGenerationStatusFilter(event.target.value as GenerationStatusFilter)}
                      value={generationStatusFilter}
                    >
                      {generationStatusFilterOptions.map((option) => (
                        <option key={option.value} value={option.value}>{option.label}</option>
                      ))}
                    </select>
                  </label>
                </div>

                <div className="admin-table-wrap" style={{ marginTop: '24px' }}>
                  <table className="admin-table">
                    <thead>
                      <tr>
                        <th>用户</th>
                        <th>提示词</th>
                        <th>状态</th>
                        <th>尺寸</th>
                        <th>耗时</th>
                        <th>时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredGenerationTasks.map((task) => (
                        <tr key={task.id}>
                          <td><strong>{task.username}</strong></td>
                          <td style={{ maxWidth: '300px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{task.prompt}</td>
                          <td><span className={`status-badge ${task.status}`}>{taskStatusLabel(task.status)}</span></td>
                          <td>{task.size}</td>
                          <td>{formatLatency(task.latencyMs)}</td>
                          <td>{formatTime(task.createdAt)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>

              <section className="admin-section panel glass-panel animate-fade-in" aria-labelledby="audit-log-title">
                <h3 id="audit-log-title">操作记录</h3>
                <div className="admin-table-wrap">
                  <table className="admin-table">
                    <thead>
                      <tr>
                        <th>动作</th>
                        <th>详情</th>
                        <th>时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {auditLogs.map((log) => (
                        <tr key={log.id}>
                          <td><strong>{log.action}</strong></td>
                          <td>{metadataText(log.metadata)}</td>
                          <td>{formatTime(log.createdAt)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </div>
          ) : null}

          {activeTab === "security" ? (
            <section className="admin-section panel glass-panel animate-fade-in" aria-labelledby="security-title">
              <h3 id="security-title">账号安全</h3>
              <form className="compact-form" onSubmit={handleOwnPasswordSubmit} style={{ maxWidth: '400px' }}>
                <label className="field">
                  <span>当前密码</span>
                  <input
                    autoComplete="current-password"
                    onChange={(event) => setOwnPasswordDraft((current) => ({ ...current, currentPassword: event.target.value }))}
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
                    onChange={(event) => setOwnPasswordDraft((current) => ({ ...current, newPassword: event.target.value }))}
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
                    onChange={(event) => setOwnPasswordDraft((current) => ({ ...current, confirmPassword: event.target.value }))}
                    required
                    type="password"
                    value={ownPasswordDraft.confirmPassword}
                  />
                </label>
                <button className="primary-button" disabled={busy === "own-password"} type="submit">
                  更新管理员密码
                </button>
              </form>
            </section>
          ) : null}
        </>
      )}
    </section>
  );
}
