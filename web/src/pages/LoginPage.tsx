import { FormEvent, useState } from "react";

import { api, normalizeAuthResponse, type User } from "../api/client";

type LoginPageProps = {
  onLogin?: (user: User) => void;
  onRegisterClick?: () => void;
};

const showcaseImages = [
  { alt: "罗威纳展示图", src: "/showcase/罗威纳.jpg" },
  { alt: "伯恩山展示图", src: "/showcase/伯恩山.jpg" },
  { alt: "恭王府展示图", src: "/showcase/恭王府.jpg" },
  { alt: "陈平安展示图", src: "/showcase/陈平安.jpg" },
  { alt: "左右展示图", src: "/showcase/左右.jpg" },
  { alt: "起床展示图", src: "/showcase/起床.jpg" },
];

function ShowcaseGallery() {
  return (
    <section className="login-showcase" aria-label="生成效果展示">
      <div className="showcase-grid">
        {showcaseImages.map((image) => (
          <img
            className="showcase-image"
            key={image.src}
            src={image.src}
            alt={image.alt}
            loading="lazy"
            decoding="async"
          />
        ))}
      </div>
    </section>
  );
}

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
    <section className="login-home" aria-label="账号登录">
      <section className="auth-surface login-card" aria-labelledby="login-title">
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
      <ShowcaseGallery />
    </section>
  );
}
