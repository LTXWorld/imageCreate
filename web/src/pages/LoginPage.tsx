import { FormEvent, useState } from "react";
import { api, normalizeAuthResponse, type User } from "../api/client";
import "../styles/Auth.css";
import "../styles/Components.css";

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
      <div className="section-heading">
         <p className="eyebrow">灵感画廊</p>
         <h2 style={{ color: 'var(--color-jade-deep)' }}>探索 AI 的无限可能</h2>
      </div>
      <div className="showcase-grid">
        {showcaseImages.map((image, i) => (
          <img
            className="showcase-image"
            key={image.src}
            src={image.src}
            alt={image.alt}
            loading="lazy"
            decoding="async"
            style={{ animationDelay: `${i * 0.1}s` }}
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
    <div className="startup-shell">
      <div className="login-home animate-fade-in">
        <section className="auth-surface login-card panel glass-panel" aria-labelledby="login-title">
          <div className="section-heading">
            <p className="eyebrow">欢迎回来</p>
            <h2 id="login-title">登录账号</h2>
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
                placeholder="请输入用户名"
                disabled={loading}
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
                placeholder="请输入密码"
                disabled={loading}
              />
            </label>

            {error ? <p className="form-error" role="alert">{error}</p> : null}

            <div className="form-actions">
              <button className="primary-button" disabled={loading} type="submit">
                {loading ? "登录中..." : "立即登录"}
              </button>
              <button className="secondary-button" type="button" onClick={onRegisterClick} disabled={loading}>
                没有账号？去注册
              </button>
            </div>
          </form>
        </section>
        <ShowcaseGallery />
      </div>
    </div>
  );
}
