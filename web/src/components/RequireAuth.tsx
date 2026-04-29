import { type ReactNode, useEffect, useState } from "react";

import { api, normalizeAuthResponse, type User } from "../api/client";

type RequireAuthProps = {
  children: ReactNode;
  user: User | null;
  onAuthenticated: (user: User) => void;
  onUnauthenticated: () => void;
};

export function RequireAuth({
  children,
  user,
  onAuthenticated,
  onUnauthenticated,
}: RequireAuthProps) {
  const [status, setStatus] = useState<"checking" | "ready" | "guest">(
    user ? "ready" : "checking",
  );

  useEffect(() => {
    let active = true;

    if (user) {
      setStatus("ready");
      return;
    }

    setStatus("checking");
    api<unknown>("/api/auth/me")
      .then((body) => {
        if (!active) return;
        const { user: currentUser } = normalizeAuthResponse(body as Parameters<typeof normalizeAuthResponse>[0]);
        onAuthenticated(currentUser);
        setStatus("ready");
      })
      .catch(() => {
        if (!active) return;
        onUnauthenticated();
        setStatus("guest");
      });

    return () => {
      active = false;
    };
  }, [onAuthenticated, onUnauthenticated, user]);

  if (status === "checking") {
    return <section className="panel status-panel">正在确认登录状态...</section>;
  }

  if (status === "guest") {
    return (
      <section className="panel status-panel">
        <h2>请先登录</h2>
        <p>登录后即可进入创作台。</p>
      </section>
    );
  }

  return <>{children}</>;
}
