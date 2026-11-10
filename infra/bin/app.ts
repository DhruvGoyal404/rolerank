import { App } from "aws-cdk-lib";

import { REGION, RoleRankHostStack, RoleRankProdStack, tagProject } from "../lib/stacks.ts";

// Run by `cdk` as `node bin/app.ts` (Node 24 strips the types). With credentials, the host
// looks up the account's default VPC; without them (CI, tests, a review synth) it synthesises
// against placeholders and the template is for review only.
const app = new App();
const account = process.env.CDK_DEFAULT_ACCOUNT || undefined;
const env = { account, region: REGION };
const context = (key: string): string | undefined => {
  const value: unknown = app.node.tryGetContext(key);
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
};

const persistent = new RoleRankProdStack(app, "RoleRankProd", {
  env,
  description: "RoleRank persistent parts: backups, registries, alarm topic, deploy document, GitHub OIDC deploy role, budget",
  terminationProtection: Boolean(account),
  // rolerank-up.sh passes both from SSM; a review synth uses a placeholder address.
  alarmEmail: context("alarmEmail") ?? (account ? required("alarmEmail") : "alarms@example.invalid"),
  budgetEmail: context("budgetEmail"),
});
new RoleRankHostStack(app, "RoleRankHost", {
  env,
  offline: !account,
  description: "RoleRank host (EC2 arm64, EBS, Elastic IP): destroyed by rolerank-down.sh",
  persistent,
  hostAmi: context("rolerank:hostAmi") ?? "",
  hostAz: context("rolerank:hostAz"),
  hostType: context("rolerank:hostType"),
});
tagProject(app);

function required(key: string): never {
  throw new Error(`pass -c ${key}=<address>: a deploy without it would drop the alarm subscription`);
}
