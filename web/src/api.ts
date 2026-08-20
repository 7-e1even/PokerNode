export class APIError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const hasJSONBody = !!options.body && !(options.body instanceof FormData);
  const response = await fetch(path, {
    ...options,
    headers: {
      ...(hasJSONBody ? { "Content-Type": "application/json" } : {}),
      ...options.headers,
    },
    credentials: "same-origin",
  });
  if (response.status === 204) return undefined as T;
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new APIError(payload.error || `请求失败 (${response.status})`, response.status);
  return payload as T;
}

export function post<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, { method: "POST", body: JSON.stringify(body ?? {}) });
}

export function put<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: "PUT", body: JSON.stringify(body) });
}

export function patch<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: "PATCH", body: JSON.stringify(body) });
}

export function upload<T>(path: string, body: FormData): Promise<T> {
  return api<T>(path, { method: "PUT", body });
}

export function remove(path: string): Promise<void> {
  return api<void>(path, { method: "DELETE" });
}
