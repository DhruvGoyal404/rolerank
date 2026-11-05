import { defineConfig } from "vitest/config";

// Live evals (tier T1): real model, real sandbox. Run by scripts/eval-live.sh, never in CI.
export default defineConfig({
  test: {
    include: ["test/**/*.eval.test.ts"],
    environment: "node",
    fileParallelism: false,
    hookTimeout: 120_000,
  },
});
