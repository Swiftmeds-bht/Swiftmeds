import { callSweep, type Env } from "./sweep";

export type { Env };

/** How many hold IDs one log line lists. The count is always complete. */
const MAX_LOGGED_IDS = 20;

export default {
  /** Runs on the cron schedule in wrangler.toml. */
  async scheduled(controller: ScheduledController, env: Env): Promise<void> {
    const run = {
      cron: controller.cron,
      scheduledTime: new Date(controller.scheduledTime).toISOString(),
    };
    try {
      const expiredHoldIds = await callSweep(env);
      console.log(
        JSON.stringify({
          level: "info",
          msg: "hold expiry sweep done",
          expiredCount: expiredHoldIds.length,
          expiredHoldIds: expiredHoldIds.slice(0, MAX_LOGGED_IDS),
          ...run,
        }),
      );
    } catch (error) {
      console.error(
        JSON.stringify({
          level: "error",
          msg: "hold expiry sweep failed",
          error: error instanceof Error ? error.message : String(error),
          ...run,
        }),
      );
      // Rethrow so Cloudflare records this run as failed. The next run, a minute later, tries again.
      throw error;
    }
  },
} satisfies ExportedHandler<Env>;
