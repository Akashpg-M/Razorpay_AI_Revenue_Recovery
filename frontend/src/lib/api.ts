export const serverBackend = process.env.BACKEND_INTERNAL_URL ?? process.env.NEXT_PUBLIC_BACKEND_URL ?? "http://localhost:8080";
export const publicBackend = process.env.NEXT_PUBLIC_BACKEND_URL ?? "http://localhost:8080";

export async function backendJSON<T>(path: string): Promise<T | null> {
  try {
    const response = await fetch(`${serverBackend}${path}`, { cache: "no-store" });
    if (!response.ok) return null;
    return (await response.json()) as T;
  } catch { return null; }
}
