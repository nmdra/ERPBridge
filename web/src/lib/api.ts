const capabilityHeader = "X-ERPBridge-Console-Capability";
const capabilityStorageKey = "erpbridge-console-capability";
let capability: string | null = null;

function readStoredCapability() {
  try {
    return window.sessionStorage.getItem(capabilityStorageKey);
  } catch {
    return null;
  }
}

function storeCapability(value: string) {
  try {
    window.sessionStorage.setItem(capabilityStorageKey, value);
  } catch {
    // Some privacy modes disable storage. The URL fragment still works.
  }
}

function readCapability() {
  if (capability !== null) {
    return capability;
  }
  const value = new URLSearchParams(window.location.hash.replace(/^#/, "")).get(
    "cap",
  );
  capability = value ?? readStoredCapability();
  if (value) {
    storeCapability(value);
    window.history.replaceState(
      null,
      document.title,
      window.location.pathname + window.location.search,
    );
  }
  return capability;
}

export async function apiFetch<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  const value = readCapability();
  if (value) {
    headers.set(capabilityHeader, value);
  }
  const response = await fetch(path, { ...init, headers });
  if (!response.ok) {
    throw new Error(`Console request failed with status ${response.status}`);
  }
  return (await response.json()) as T;
}

export function consoleCapability(): string | null {
  return readCapability();
}

export async function streamLogEvents(
  path: string,
  signal: AbortSignal,
  onEvent: (event: unknown) => void,
): Promise<void> {
  const headers = new Headers({ Accept: "text/event-stream" });
  const value = readCapability();
  if (value) {
    headers.set(capabilityHeader, value);
  }
  const response = await fetch(path, { headers, signal });
  if (!response.ok) {
    throw new Error(`Console stream failed with status ${response.status}`);
  }
  if (!response.body) {
    throw new Error("Console stream has no body");
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (true) {
      const result = await reader.read();
      if (result.done) break;
      buffer += decoder.decode(result.value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";
      for (const line of lines) {
        if (!line.startsWith("data: ")) continue;
        try {
          onEvent(JSON.parse(line.slice(6)));
        } catch {
          // Ignore malformed events. The BFF also filters malformed events.
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}
