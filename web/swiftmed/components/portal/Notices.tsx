/** The ?error= and ?notice= messages that actions redirect with. */
export function Notices({ error, notice }: { error?: string; notice?: string }) {
  return (
    <>
      {error ? (
        <p className="message message-error" role="alert">
          {error}
        </p>
      ) : null}
      {notice ? (
        <p className="message message-success" role="status">
          {notice}
        </p>
      ) : null}
    </>
  );
}

export type SearchParams = Promise<Record<string, string | string[] | undefined>>;

export function queryText(value: string | string[] | undefined): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}
