import { FormEvent, useState } from "react";

import { api, normalizeAuthResponse, type User } from "../api/client";

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
    <section className="auth-surface" aria-labelledby="register-title">
      <div className="section-heading">
        <p className="eyebrow">邀请访问</p>
        <h2 id="register-title">注册</h2>
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
          />
        </label>

        {error ? <p className="form-error" role="alert">{error}</p> : null}

        <div className="form-actions">
          <button className="primary-button" disabled={loading} type="submit">
            {loading ? "注册中..." : "注册"}
          </button>
          <button className="secondary-button" type="button" onClick={onLoginClick}>
            去登录
          </button>
        </div>
      </form>
    </section>
  );
}
