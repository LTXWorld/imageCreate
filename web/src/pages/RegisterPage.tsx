import { FormEvent, useState } from "react";
import { api, normalizeAuthResponse, type User } from "../api/client";
import "../styles/Auth.css";
import "../styles/Components.css";

type RegisterPageProps = {
  onRegister?: (user: User) => void;
  onLoginClick?: () => void;
};

export function RegisterPage({ onRegister, onLoginClick }: RegisterPageProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [inviteCode, setInviteCode] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setLoading(true);

    try {
      const body = await api<unknown>("/api/auth/register", {
        method: "POST",
        body: JSON.stringify({
          username,
          password,
          invite_code: inviteCode,
        }),
      });
      const { user } = normalizeAuthResponse(body as Parameters<typeof normalizeAuthResponse>[0]);
      onRegister?.(user);
    } catch (err) {
      setError(err instanceof Error ? err.message : "注册失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="startup-shell">
      <section className="auth-surface panel glass-panel animate-fade-in" aria-labelledby="register-title">
        <div className="section-heading">
          <p className="eyebrow">开启创作之旅</p>
          <h2 id="register-title">注册新账号</h2>
        </div>

        <form className="auth-form" onSubmit={handleSubmit}>
          <label className="field">
            <span>用户名</span>
            <input
              autoComplete="username"
              name="username"
              onChange={(event) => setUsername(event.target.value)}
              required
              value={username}
              placeholder="请设置用户名"
              disabled={loading}
            />
          </label>

          <label className="field">
            <span>密码</span>
            <input
              autoComplete="new-password"
              name="password"
              onChange={(event) => setPassword(event.target.value)}
              required
              type="password"
              value={password}
              placeholder="请设置密码"
              disabled={loading}
            />
          </label>

          <label className="field">
            <span>邀请码</span>
            <input
              autoComplete="off"
              name="inviteCode"
              onChange={(event) => setInviteCode(event.target.value)}
              required
              value={inviteCode}
              placeholder="请输入邀请码"
              disabled={loading}
            />
          </label>

          {error ? <p className="form-error" role="alert">{error}</p> : null}

          <div className="form-actions">
            <button className="primary-button" disabled={loading} type="submit">
              {loading ? "注册中..." : "立即注册"}
            </button>
            <button className="secondary-button" type="button" onClick={onLoginClick} disabled={loading}>
              已有账号？去登录
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
