export type User = {
  id: string;
  username: string;
  role: "user" | "admin";
  status: "active" | "disabled";
  creditBalance: number;
};

type ApiUser = User & {
  credit_balance?: number;
};

export type AuthResponse = {
  user: User;
};

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(path, {
    ...init,
    credentials: "include",
    headers,
  });
  if (!response.ok) {
    const body = await response
      .json()
      .catch(() => ({ message: "请求失败" }));
    throw new Error(body.message ?? body.error ?? "请求失败");
  }
  return response.json() as Promise<T>;
}

export function normalizeUser(user: ApiUser): User {
  return {
    id: user.id,
    username: user.username,
    role: user.role,
    status: user.status,
    creditBalance: user.creditBalance ?? user.credit_balance ?? 0,
  };
}

export function normalizeAuthResponse(body: ApiUser | { user: ApiUser }): AuthResponse {
  const user = "user" in body ? body.user : body;
  return { user: normalizeUser(user) };
}
