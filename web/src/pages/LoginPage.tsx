import { FormEvent, useState } from "react";

import { api, normalizeAuthResponse, type User } from "../api/client";

type LoginPageProps = {
  onLogin?: (user: User) => void;
  onRegisterClick?: () => void;
};

export function LoginPage({ onLogin, onRegisterClick }: LoginPageProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setLoading(true);

    try {
      const body = await api<unknown>("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      });
      const { user } = normalizeAuthResponse(body as Parameters<typeof normalizeAuthResponse>[0]);
      onLogin?.(user);
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="auth-surface" aria-labelledby="login-title">
      <div className="section-heading">
        <p className="eyebrow">已有账号</p>
        <h2 id="login-title">登录</h2>
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
            autoComplete="current-password"
            name="password"
            onChange={(event) => setPassword(event.target.value)}
            required
            type="password"
            value={password}
          />
        </label>

        {error ? <p className="form-error" role="alert">{error}</p> : null}

        <div className="form-actions">
          <button className="primary-button" disabled={loading} type="submit">
            {loading ? "登录中..." : "登录"}
          </button>
          <button className="secondary-button" type="button" onClick={onRegisterClick}>
            去注册
          </button>
        </div>
      </form>
    </section>
  );
}
