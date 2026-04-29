import { ImagePlus, LogOut, ShieldCheck, UserRound } from "lucide-react";
import type { ReactNode } from "react";

import type { User } from "../api/client";

type LayoutProps = {
  children: ReactNode;
  user: User | null;
  activeView: "login" | "register" | "workspace" | "admin";
  onNavigate: (view: "login" | "register" | "workspace" | "admin") => void;
  onLogout?: () => void;
};

export function Layout({
  children,
  user,
  activeView,
  onNavigate,
  onLogout,
}: LayoutProps) {
  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="主导航">
        <div className="brand">
          <span className="brand-mark">
            <ImagePlus size={20} aria-hidden="true" />
          </span>
          <span>AI 生图</span>
        </div>

        <nav className="nav-list">
          <button
            className={activeView === "workspace" ? "nav-item active" : "nav-item"}
            type="button"
            onClick={() => onNavigate("workspace")}
          >
            <ImagePlus size={18} aria-hidden="true" />
            <span>创作台</span>
          </button>
          {user?.role === "admin" ? (
            <button
              className={activeView === "admin" ? "nav-item active" : "nav-item"}
              type="button"
              onClick={() => onNavigate("admin")}
            >
              <ShieldCheck size={18} aria-hidden="true" />
              <span>后台</span>
            </button>
          ) : null}
          {!user ? (
            <>
              <button
                className={activeView === "login" ? "nav-item active" : "nav-item"}
                type="button"
                onClick={() => onNavigate("login")}
              >
                <UserRound size={18} aria-hidden="true" />
                <span>登录</span>
              </button>
              <button
                className={activeView === "register" ? "nav-item active" : "nav-item"}
                type="button"
                onClick={() => onNavigate("register")}
              >
                <ShieldCheck size={18} aria-hidden="true" />
                <span>注册</span>
              </button>
            </>
          ) : null}
        </nav>
      </aside>

      <div className="main-column">
        <header className="topbar">
          <div>
            <p className="eyebrow">图像生成工作台</p>
            <h1>账号访问</h1>
          </div>
          <div className="account-strip">
            {user ? (
              <>
                <span className="account-name">{user.username}</span>
                <span className="credit-pill">{user.creditBalance} 点</span>
                <button className="icon-button" type="button" onClick={onLogout} aria-label="退出登录">
                  <LogOut size={18} aria-hidden="true" />
                </button>
              </>
            ) : (
              <span className="muted-text">未登录</span>
            )}
          </div>
        </header>
        <main className="content">{children}</main>
      </div>
    </div>
  );
}
