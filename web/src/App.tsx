import { useCallback, useEffect, useState } from "react";

import { api, normalizeAuthResponse, type User } from "./api/client";
import { Layout } from "./components/Layout";
import { RequireAuth } from "./components/RequireAuth";
import { AdminPage } from "./pages/AdminPage";
import { HistoryPage } from "./pages/HistoryPage";
import { LoginPage } from "./pages/LoginPage";
import { RegisterPage } from "./pages/RegisterPage";
import { WorkspacePage } from "./pages/WorkspacePage";
import "./styles/app.css";

type View = "login" | "register" | "workspace" | "history" | "admin";

export function App() {
  const [view, setView] = useState<View>("login");
  const [user, setUser] = useState<User | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);

  const refreshCurrentUser = useCallback(async () => {
    const body = await api<unknown>("/api/auth/me");
    const { user: currentUser } = normalizeAuthResponse(
      body as Parameters<typeof normalizeAuthResponse>[0],
    );
    setUser(currentUser);
    return currentUser;
  }, []);

  useEffect(() => {
    let active = true;

    api<unknown>("/api/auth/me")
      .then((body) => {
        if (!active) return;
        const { user: currentUser } = normalizeAuthResponse(
          body as Parameters<typeof normalizeAuthResponse>[0],
        );
        setUser(currentUser);
        setView(currentUser.role === "admin" ? "admin" : "workspace");
      })
      .catch(() => {
        if (!active) return;
        setUser(null);
        setView("login");
      })
      .finally(() => {
        if (!active) return;
        setCheckingSession(false);
      });

    return () => {
      active = false;
    };
  }, []);

  const handleAuthenticated = useCallback((currentUser: User) => {
    setUser(currentUser);
    setView(currentUser.role === "admin" ? "admin" : "workspace");
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

  if (checkingSession) {
    return (
      <main className="startup-shell">
        <section className="panel status-panel">正在确认登录状态...</section>
      </main>
    );
  }

  const content = view === "register" ? (
    <RegisterPage onRegister={handleAuthenticated} onLoginClick={() => setView("login")} />
  ) : view === "admin" ? (
    <RequireAuth
      user={user}
      onAuthenticated={handleAuthenticated}
      onUnauthenticated={handleUnauthenticated}
    >
      {user ? <AdminPage user={user} /> : null}
    </RequireAuth>
  ) : view === "history" ? (
    <RequireAuth
      user={user}
      onAuthenticated={handleAuthenticated}
      onUnauthenticated={handleUnauthenticated}
    >
      <HistoryPage onWorkspaceClick={() => setView("workspace")} />
    </RequireAuth>
  ) : view === "workspace" ? (
    <RequireAuth
      user={user}
      onAuthenticated={handleAuthenticated}
      onUnauthenticated={handleUnauthenticated}
    >
      {user ? (
        <WorkspacePage
          user={user}
          onHistoryClick={() => setView("history")}
          onUserRefresh={refreshCurrentUser}
        />
      ) : null}
    </RequireAuth>
  ) : (
    <LoginPage onLogin={handleAuthenticated} onRegisterClick={() => setView("register")} />
  );

  return (
    <Layout
      user={user}
      activeView={view === "history" ? "workspace" : view}
      onNavigate={(nextView) => setView(nextView)}
      onLogout={handleLogout}
    >
      {content}
    </Layout>
  );
}
