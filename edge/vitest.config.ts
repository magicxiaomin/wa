import { defineWorkersConfig } from "@cloudflare/vitest-pool-workers/config";

export default defineWorkersConfig({
  test: {
    poolOptions: {
      workers: {
        wrangler: { configPath: "./wrangler.toml" },
        isolatedStorage: false,
        miniflare: {
          bindings: {
            RELAY_TOKEN: "test-token-1234567890-1234567890",
            RELAY_TIMEOUT_MS: "50"
          }
        }
      }
    }
  }
});
