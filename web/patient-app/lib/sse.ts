// Reads a server-sent events stream (text/event-stream) as it arrives.
// Used on the server to read the gateway's alert stream (SCRUM-71).

export interface SSEEvent {
  /** "message" when the stream gives no `event:` line. */
  event: string;
  data: string;
  id?: string;
}

/**
 * Yields one event per blank-line-separated block. Comment lines (starting with
 * ":", such as the gateway's heartbeats) are skipped, and several `data:` lines
 * are joined with newlines, as the SSE specification says.
 */
export async function* parseSSE(body: ReadableStream<Uint8Array>): AsyncGenerator<SSEEvent> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      // stream: true keeps a character split across chunks whole.
      buffer += decoder.decode(value, { stream: true }).replace(/\r\n?/g, "\n");
      let end: number;
      while ((end = buffer.indexOf("\n\n")) !== -1) {
        const event = parseBlock(buffer.slice(0, end));
        buffer = buffer.slice(end + 2);
        if (event) yield event;
      }
    }
  } finally {
    reader.releaseLock();
  }
}

function parseBlock(block: string): SSEEvent | undefined {
  let event = "message";
  let id: string | undefined;
  const data: string[] = [];
  for (const line of block.split("\n")) {
    if (line === "" || line.startsWith(":")) continue;
    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    const value = colon === -1 ? "" : line.slice(colon + 1).replace(/^ /, "");
    if (field === "event") event = value;
    else if (field === "data") data.push(value);
    else if (field === "id") id = value;
  }
  if (data.length === 0) return undefined;
  return id === undefined ? { event, data: data.join("\n") } : { event, data: data.join("\n"), id };
}

/** One event in SSE form, ready to write to a response. */
export function formatSSE(event: string, data: unknown, id?: string): string {
  const lines = id ? [`id: ${id}`] : [];
  lines.push(`event: ${event}`, `data: ${JSON.stringify(data)}`);
  return lines.join("\n") + "\n\n";
}
