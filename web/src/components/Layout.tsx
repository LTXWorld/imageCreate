import { ImagePlus, LogOut, ShieldCheck } from "lucide-react";
import type { ReactNode } from "react";

import type { User } from "../api/client";
import "../styles/Layout.css";
import "../styles/Components.css";

type LayoutProps = {
  children: ReactNode;
  user: User | null;
  activeView: "landing" | "login" | "register" | "workspace" | "admin" | "history";
  onNavigate: (view: "landing" | "login" | "register" | "workspace" | "admin" | "history") => void;
  onLogout?: () => void;
};

export function Layout({
  children,
  user,
  activeView,
  onNavigate,
  onLogout,
}: LayoutProps) {
  const isAuthView = activeView === "landing" || activeView === "login" || activeView === "register";

  if (isAuthView) {
    return <main>{children}</main>;
  }

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
        </nav>
        
        <div style={{ marginTop: 'auto' }}>
           <button className="nav-item" type="button" onClick={onLogout}>
              <LogOut size={18} aria-hidden="true" />
              <span>退出登录</span>
           </button>
        </div>
      </aside>

      <div className="main-column">
        <header className="topbar">
          <div className="animate-fade-in">
            <p className="eyebrow">图像生成工作台</p>
            <h1>{activeView === "workspace" ? "创作中心" : activeView === "admin" ? "管理后台" : "历史记录"}</h1>
          </div>
          <div className="account-strip animate-fade-in">
            {user ? (
              <>
                <span className="account-name">{user.username}</span>
                <span className="credit-pill">{user.creditBalance} 点</span>
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
