export type User = {
  id: string;
  username: string;
  role: "user" | "admin";
  status: "active" | "disabled";
  creditBalance: number;
  dailyFreeCreditLimit: number;
  dailyFreeCreditBalance: number;
  paidCreditBalance: number;
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

export type AdminUser = User & {
  createdAt: string;
  updatedAt: string;
};

export type AdminInvite = {
  id: string;
  code: string;
  initialCredits: number;
  status: "unused" | "used";
  createdBy?: string;
  usedBy?: string;
  usedAt?: string;
  createdAt: string;
};

export type AdminAuditLog = {
  id: string;
  actorUserId?: string;
  targetUserId?: string;
  action: string;
  metadata: unknown;
  createdAt: string;
};

export type AdminGenerationTask = {
  id: string;
  userId: string;
  username: string;
  prompt: string;
  size: string;
  status: GenerationTask["status"];
  latencyMs: number;
  errorCode?: string;
  errorMessage?: string;
  createdAt: string;
  completedAt?: string;
};

type ApiUser = {
  id: string;
  username: string;
  role: User["role"];
  status: User["status"];
  creditBalance?: number;
  credit_balance?: number;
  daily_free_credit_limit?: number;
  dailyFreeCreditLimit?: number;
  daily_free_credit_balance?: number;
  dailyFreeCreditBalance?: number;
  paid_credit_balance?: number;
  paidCreditBalance?: number;
  created_at?: string;
  createdAt?: string;
  updated_at?: string;
  updatedAt?: string;
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

type WrappedGenerationTask = {
  task?: ApiGenerationTask;
};

type ApiInvite = {
  id: string;
  code: string;
  initial_credits?: number;
  initialCredits?: number;
  status: "unused" | "used";
  created_by?: string;
  createdBy?: string;
  used_by?: string;
  usedBy?: string;
  used_at?: string;
  usedAt?: string;
  created_at?: string;
  createdAt?: string;
};

type ApiAuditLog = {
  id: string;
  actor_user_id?: string;
  actorUserId?: string;
  target_user_id?: string;
  targetUserId?: string;
  action: string;
  metadata: unknown;
  created_at?: string;
  createdAt?: string;
};

type ApiAdminGenerationTask = {
  id: string;
  user_id?: string;
  userId?: string;
  username: string;
  prompt: string;
  size: string;
  status: GenerationTask["status"];
  latency_ms?: number;
  latencyMs?: number;
  error_code?: string;
  errorCode?: string;
  error_message?: string;
  errorMessage?: string;
  created_at?: string;
  createdAt?: string;
  completed_at?: string;
  completedAt?: string;
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
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export function normalizeUser(user: ApiUser): User {
  const explicitCreditBalance = user.creditBalance ?? user.credit_balance;
  const hasSplitWalletFields =
    user.dailyFreeCreditLimit !== undefined ||
    user.daily_free_credit_limit !== undefined ||
    user.dailyFreeCreditBalance !== undefined ||
    user.daily_free_credit_balance !== undefined ||
    user.paidCreditBalance !== undefined ||
    user.paid_credit_balance !== undefined;
  const legacyCreditBalance = explicitCreditBalance ?? 0;
  const dailyFreeCreditLimit = user.dailyFreeCreditLimit ?? user.daily_free_credit_limit ?? (
    hasSplitWalletFields ? 0 : legacyCreditBalance
  );
  const dailyFreeCreditBalance = user.dailyFreeCreditBalance ?? user.daily_free_credit_balance ?? (
    hasSplitWalletFields ? 0 : legacyCreditBalance
  );
  const paidCreditBalance = user.paidCreditBalance ?? user.paid_credit_balance ?? 0;

  return {
    id: user.id,
    username: user.username,
    role: user.role,
    status: user.status,
    creditBalance: explicitCreditBalance ?? dailyFreeCreditBalance + paidCreditBalance,
    dailyFreeCreditLimit,
    dailyFreeCreditBalance,
    paidCreditBalance,
  };
}

export function normalizeAdminUser(user: ApiUser): AdminUser {
  return {
    ...normalizeUser(user),
    createdAt: user.createdAt ?? user.created_at ?? "",
    updatedAt: user.updatedAt ?? user.updated_at ?? "",
  };
}

export function normalizeAuthResponse(body: ApiUser | { user: ApiUser }): AuthResponse {
  const user = "user" in body ? body.user : body;
  return { user: normalizeUser(user) };
}

export function normalizeGenerationTask(body: ApiGenerationTask | WrappedGenerationTask): GenerationTask {
  const task = "task" in body && body.task ? body.task : body as ApiGenerationTask;
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

export function generationImageFilename(task: Pick<GenerationTask, "id" | "ratio">): string {
  const safeRatio = task.ratio.replace(/[^a-zA-Z0-9]+/g, "-").replace(/^-|-$/g, "");
  return `imagecreate-${task.id}-${safeRatio || "image"}.png`;
}

export function normalizeAdminUsers(body: unknown): AdminUser[] {
  const source = typeof body === "object" && body !== null && "users" in body
    ? (body as { users: unknown }).users
    : [];
  return Array.isArray(source) ? source.map((user) => normalizeAdminUser(user as ApiUser)) : [];
}

export function normalizeAdminInvites(body: unknown): AdminInvite[] {
  const source = typeof body === "object" && body !== null && "invites" in body
    ? (body as { invites: unknown }).invites
    : [];

  return Array.isArray(source)
    ? source.map((invite) => {
      const item = invite as ApiInvite;
      return {
        id: item.id,
        code: item.code,
        initialCredits: item.initialCredits ?? item.initial_credits ?? 0,
        status: item.status,
        createdBy: item.createdBy ?? item.created_by,
        usedBy: item.usedBy ?? item.used_by,
        usedAt: item.usedAt ?? item.used_at,
        createdAt: item.createdAt ?? item.created_at ?? "",
      };
    })
    : [];
}

export function normalizeAdminAuditLogs(body: unknown): AdminAuditLog[] {
  const source = typeof body === "object" && body !== null && "audit_logs" in body
    ? (body as { audit_logs: unknown }).audit_logs
    : [];

  return Array.isArray(source)
    ? source.map((log) => {
      const item = log as ApiAuditLog;
      return {
        id: item.id,
        actorUserId: item.actorUserId ?? item.actor_user_id,
        targetUserId: item.targetUserId ?? item.target_user_id,
        action: item.action,
        metadata: item.metadata,
        createdAt: item.createdAt ?? item.created_at ?? "",
      };
    })
    : [];
}

export function normalizeAdminGenerationTasks(body: unknown): AdminGenerationTask[] {
  const source = typeof body === "object" && body !== null && "tasks" in body
    ? (body as { tasks: unknown }).tasks
    : [];

  return Array.isArray(source)
    ? source.map((task) => {
      const item = task as ApiAdminGenerationTask;
      return {
        id: item.id,
        userId: item.userId ?? item.user_id ?? "",
        username: item.username,
        prompt: item.prompt,
        size: item.size,
        status: item.status,
        latencyMs: item.latencyMs ?? item.latency_ms ?? 0,
        errorCode: item.errorCode ?? item.error_code,
        errorMessage: item.errorMessage ?? item.error_message,
        createdAt: item.createdAt ?? item.created_at ?? "",
        completedAt: item.completedAt ?? item.completed_at,
      };
    })
    : [];
}
