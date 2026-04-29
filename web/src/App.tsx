import { useCallback, useState } from "react";

import { api, type User } from "./api/client";
import { Layout } from "./components/Layout";
import { RequireAuth } from "./components/RequireAuth";
import { LoginPage } from "./pages/LoginPage";
import { RegisterPage } from "./pages/RegisterPage";
import "./styles/app.css";

export function App() {
  const [view, setView] = useState<"login" | "register" | "workspace">("login");
  const [user, setUser] = useState<User | null>(null);

  const handleAuthenticated = useCallback((currentUser: User) => {
    setUser(currentUser);
    setView("workspace");
  }, []);

  const handleUnauthenticated = useCallback(() => {
    setUser(null);
    setView("login");
  }, []);

  async function handleLogout() {
    await api<{ ok: boolean }>("/api/auth/logout", { method: "POST" }).catch(() => ({ ok: false }));
    setUser(null);
    setView("login");
  }

  const content =
    view === "register" ? (
      <RegisterPage onRegister={handleAuthenticated} onLoginClick={() => setView("login")} />
    ) : view === "workspace" ? (
      <RequireAuth
        user={user}
        onAuthenticated={handleAuthenticated}
        onUnauthenticated={handleUnauthenticated}
      >
        <section className="workspace-panel">
          <div className="section-heading">
            <p className="eyebrow">创作台</p>
            <h2>图像生成</h2>
          </div>
          <div className="empty-workspace">
            <p>生成表单将在后续任务接入。</p>
          </div>
        </section>
      </RequireAuth>
    ) : (
      <LoginPage onLogin={handleAuthenticated} onRegisterClick={() => setView("register")} />
    );

  return (
    <Layout user={user} activeView={view} onNavigate={setView} onLogout={handleLogout}>
      {content}
    </Layout>
  );
}
