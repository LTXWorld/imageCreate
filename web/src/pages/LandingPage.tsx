import { ArrowRight, ImagePlus, LogIn, Sparkles, Ticket } from "lucide-react";

import "../styles/Components.css";
import "../styles/Landing.css";

type LandingPageProps = {
  onLoginClick?: () => void;
  onRegisterClick?: () => void;
};

const fakaUrl = import.meta.env.VITE_FAKA_URL?.trim() || "https://faka.bfsmlt.com/";

const showcaseImages = [
  { alt: "罗威纳展示图", src: "/showcase/罗威纳.jpg" },
  { alt: "伯恩山展示图", src: "/showcase/伯恩山.jpg" },
  { alt: "恭王府展示图", src: "/showcase/恭王府.jpg" },
  { alt: "陈平安展示图", src: "/showcase/陈平安.jpg" },
  { alt: "左右展示图", src: "/showcase/左右.jpg" },
  { alt: "起床展示图", src: "/showcase/起床.jpg" },
];

const features = [
  "中文提示词一键生成图片",
  "支持文生图与图生图创作",
  "最近 30 天历史记录可下载",
  "邀请码注册，小范围稳定开放",
];

export function LandingPage({ onLoginClick, onRegisterClick }: LandingPageProps) {
  return (
    <main className="landing-shell">
      <section className="landing-hero animate-fade-in" aria-labelledby="landing-title">
        <div className="landing-copy">
          <div className="landing-brand" aria-label="AI 生图工作台">
            <span className="landing-brand-mark">
              <ImagePlus size={22} aria-hidden="true" />
            </span>
            <span>AI 生图工作台</span>
          </div>

          <p className="eyebrow">邀测开放中</p>
          <h1 id="landing-title">用中文描述灵感，让 AI 为你生成精美图片</h1>
          <p className="landing-subtitle">
            面向小范围用户的 AI 文生图 / 图生图工具。购买邀请码或额度后，即可注册账号并开始创作。
          </p>

          <div className="landing-actions" aria-label="首页操作">
            <a className="primary-button landing-buy-button" href={fakaUrl} target="_blank" rel="noreferrer">
              <Ticket size={18} aria-hidden="true" />
              购买邀请码 / 额度
              <ArrowRight size={18} aria-hidden="true" />
            </a>
            <button className="secondary-button" type="button" onClick={onLoginClick}>
              <LogIn size={18} aria-hidden="true" />
              已有账号登录
            </button>
          </div>

          <button className="landing-register-link" type="button" onClick={onRegisterClick}>
            已有邀请码？立即注册账号
          </button>

          <ul className="landing-feature-list" aria-label="产品能力">
            {features.map((feature) => (
              <li key={feature}>
                <Sparkles size={16} aria-hidden="true" />
                <span>{feature}</span>
              </li>
            ))}
          </ul>
        </div>

        <div className="landing-gallery-wrap panel glass-panel">
          <div className="landing-gallery-heading">
            <p className="eyebrow">生成效果展示</p>
            <h2>看看可以创作什么</h2>
          </div>
          <div className="landing-showcase-grid" aria-label="生成效果展示">
            {showcaseImages.map((image, index) => (
              <img
                key={image.src}
                className="landing-showcase-image"
                src={image.src}
                alt={image.alt}
                loading="lazy"
                decoding="async"
                style={{ animationDelay: `${index * 0.08}s` }}
              />
            ))}
          </div>
        </div>
      </section>
    </main>
  );
}
