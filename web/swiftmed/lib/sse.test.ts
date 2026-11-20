import { describe, expect, it } from "vitest";

import { formatSSE, parseSSE, type SSEEvent } from "./sse";

function streamOf(...chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c));
      controller.close();
    },
  });
}

async function collect(body: ReadableStream<Uint8Array>): Promise<SSEEvent[]> {
  const out: SSEEvent[] = [];
  for await (const e of parseSSE(body)) out.push(e);
  return out;
}

describe("parseSSE", () => {
  it("reads the gateway's alert events and skips comments", async () => {
    const events = await collect(
      streamOf(': connected\n\nid: sub-1\nevent: alert\ndata: {"quantityAvailable":4}\n\n: ping\n\n'),
    );
    expect(events).toEqual([{ event: "alert", id: "sub-1", data: '{"quantityAvailable":4}' }]);
  });

  it("joins events split across chunks, and handles CRLF", async () => {
    const events = await collect(streamOf("event: al", "ert\r\ndata: a\r\n", "data: b\r\n\r\nda", "ta: c\n\n"));
    expect(events).toEqual([
      { event: "alert", data: "a\nb" },
      { event: "message", data: "c" },
    ]);
  });

  it("ignores a block without data and an unfinished last block", async () => {
    expect(await collect(streamOf("event: alert\n\ndata: x"))).toEqual([]);
  });
});

describe("formatSSE", () => {
  it("writes an event the parser reads back", async () => {
    const text = formatSSE("alert", { a: 1 }, "sub-1");
    expect(text).toBe('id: sub-1\nevent: alert\ndata: {"a":1}\n\n');
    expect(await collect(streamOf(text))).toEqual([{ event: "alert", id: "sub-1", data: '{"a":1}' }]);
    expect(formatSSE("error", { error: "x" })).toBe('event: error\ndata: {"error":"x"}\n\n');
  });
});
