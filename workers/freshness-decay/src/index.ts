import { callDecay, type Env } from "./decay";

export type { Env };

export default {
  /** Runs on the cron schedule in wrangler.toml. */
  async scheduled(controller: ScheduledController, env: Env): Promise<void> {
    const run = {
      cron: controller.cron,
      scheduledTime: new Date(controller.scheduledTime).toISOString(),
    };
    try {
      const updatedCount = await callDecay(env);
      console.log(JSON.stringify({ level: "info", msg: "freshness decay applied", updatedCount, ...run }));
    } catch (error) {
      console.error(
        JSON.stringify({
          level: "error",
          msg: "freshness decay failed",
          error: error instanceof Error ? error.message : String(error),
          ...run,
        }),
      );
      // Rethrow so Cloudflare records this run as failed. The next hourly run tries again.
      throw error;
    }
  },
} satisfies ExportedHandler<Env>;
