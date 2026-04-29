export type User = {
  id: string;
  username: string;
  role: "user" | "admin";
  status: "active" | "disabled";
  creditBalance: number;
};

export type GenerationTask = {
  id: string;
  prompt: string;
  ratio: string;
  size: string;
  status: "queued" | "running" | "succeeded" | "failed" | "canceled";
  imageUrl?: string;
  errorCode?: string;
  message?: string;
  createdAt: string;
  completedAt?: string;
};

type ApiUser = User & {
  credit_balance?: number;
};

type ApiGenerationTask = Omit<
  GenerationTask,
  "createdAt" | "completedAt" | "errorCode" | "imageUrl"
> & {
  created_at?: string;
  completed_at?: string;
  error_code?: string;
  error_message?: string;
  image_url?: string;
  createdAt?: string;
  completedAt?: string;
  errorCode?: string;
  imageUrl?: string;
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

export function normalizeGenerationTask(task: ApiGenerationTask): GenerationTask {
  const id = String(task.id);
  const status = task.status;
  const imageUrl =
    task.imageUrl ?? task.image_url ?? (status === "succeeded" ? `/api/generations/${id}/image` : undefined);

  return {
    id,
    prompt: task.prompt,
    ratio: task.ratio,
    size: task.size,
    status,
    imageUrl,
    errorCode: task.errorCode ?? task.error_code,
    message: task.message ?? task.error_message,
    createdAt: task.createdAt ?? task.created_at ?? "",
    completedAt: task.completedAt ?? task.completed_at,
  };
}

export function normalizeGenerationList(body: unknown): GenerationTask[] {
  const source = Array.isArray(body)
    ? body
    : typeof body === "object" && body !== null && "tasks" in body
      ? (body as { tasks: unknown }).tasks
      : typeof body === "object" && body !== null && "generations" in body
        ? (body as { generations: unknown }).generations
        : typeof body === "object" && body !== null && "items" in body
          ? (body as { items: unknown }).items
          : [];

  return Array.isArray(source)
    ? source.map((task) => normalizeGenerationTask(task as ApiGenerationTask))
    : [];
}
