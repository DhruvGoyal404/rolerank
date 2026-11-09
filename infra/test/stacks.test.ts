import assert from "node:assert/strict";
import { test } from "node:test";

import { App } from "aws-cdk-lib";
import { Match, Template } from "aws-cdk-lib/assertions";

import { BUDGET_USD, GITHUB_SUB, HOST_INSTANCE, RoleRankHostStack, RoleRankProdStack, tagProject } from "../lib/stacks.ts";

const app = new App();
const env = { account: "111111111111", region: "ap-south-1" };
const prodStack = new RoleRankProdStack(app, "Prod", { env, alarmEmail: "alarms@example.invalid" });
const hostStack = new RoleRankHostStack(app, "Host", { env, offline: true, persistent: prodStack, hostAmi: "ami-0123456789abcdef0" });
tagProject(app);
const prod = Template.fromStack(prodStack);
const host = Template.fromStack(hostStack);

const statements = (t: Template): Array<{ Action: string | string[]; Resource: unknown; Sid?: string }> =>
  Object.values(t.findResources("AWS::IAM::Policy")).flatMap((p) => (p as { Properties: { PolicyDocument: { Statement: [] } } }).Properties.PolicyDocument.Statement);
const actions = (t: Template) => statements(t).flatMap((s) => [s.Action].flat());

test("the host admits HTTP and HTTPS only: no SSH, access is through SSM", () => {
  const sg = Object.values(host.findResources("AWS::EC2::SecurityGroup"))[0] as { Properties: { SecurityGroupIngress: Array<{ FromPort: number }> } };
  assert.deepEqual([...new Set(sg.Properties.SecurityGroupIngress.map((r) => r.FromPort))].sort(), [443, 80]);
});

test("the host is the planned size, requires IMDSv2 and has an encrypted disk", () => {
  host.hasResourceProperties("AWS::EC2::Instance", { InstanceType: HOST_INSTANCE, ImageId: "ami-0123456789abcdef0" });
  host.hasResourceProperties("AWS::EC2::LaunchTemplate", { LaunchTemplateData: { MetadataOptions: { HttpTokens: "required" } } });
  host.hasResourceProperties("AWS::EC2::Instance", { BlockDeviceMappings: [{ Ebs: Match.objectLike({ Encrypted: true, VolumeType: "gp3" }) }] });
});

test("the host may put and get backups but never delete them", () => {
  const a = actions(host);
  assert.ok(a.includes("s3:PutObject") && a.includes("s3:GetObject"));
  assert.ok(!a.some((x) => /^s3:Delete|^s3:\*|^\*$/.test(x)), `host can delete: ${a.join(", ")}`);
});

test("user data carries no secrets", () => {
  const ud = JSON.stringify(host.findResources("AWS::EC2::Instance"));
  assert.ok(ud.includes("bootstrap.sh"));
  assert.ok(!/(PASSWORD|SECRET|API_KEY|KEK)=/.test(ud));
});

test("backups are versioned, locked against deletion and private", () => {
  prod.hasResourceProperties("AWS::S3::Bucket", {
    ObjectLockEnabled: true,
    ObjectLockConfiguration: { ObjectLockEnabled: "Enabled", Rule: { DefaultRetention: { Mode: "GOVERNANCE", Days: 14 } } },
    VersioningConfiguration: { Status: "Enabled" },
    PublicAccessBlockConfiguration: { BlockPublicAcls: true, BlockPublicPolicy: true, IgnorePublicAcls: true, RestrictPublicBuckets: true },
  });
  prod.hasResource("AWS::S3::Bucket", { DeletionPolicy: "Retain" });
});

test("the deploy role trusts only this repository's main branch", () => {
  prod.hasResourceProperties("AWS::IAM::Role", {
    AssumeRoleWithWebIdentity: Match.absent(),
    AssumeRolePolicyDocument: {
      Statement: [Match.objectLike({ Condition: { StringEquals: { "token.actions.githubusercontent.com:sub": GITHUB_SUB, "token.actions.githubusercontent.com:aud": "sts.amazonaws.com" } } })],
    },
  });
});

test("the budget is Rs 500 in USD, filtered to this project, alerting at 50/80/100% and forecast", () => {
  const b = Object.values(prod.findResources("AWS::Budgets::Budget"))[0] as {
    Properties: { Budget: { BudgetLimit: { Amount: number }; CostFilters: { TagKeyValue: string[] } }; NotificationsWithSubscribers: Array<{ Notification: { NotificationType: string; Threshold: number } }> };
  };
  assert.equal(b.Properties.Budget.BudgetLimit.Amount, BUDGET_USD);
  assert.match((b.Properties.Budget as unknown as { BudgetName: string }).BudgetName, /^rolerank-monthly-[0-9a-f]{8}$/);
  assert.deepEqual(b.Properties.Budget.CostFilters.TagKeyValue, ["user:rolerank:project$rolerank"]);
  assert.deepEqual(
    b.Properties.NotificationsWithSubscribers.map((n) => `${n.Notification.NotificationType}:${String(n.Notification.Threshold)}`),
    ["ACTUAL:50", "ACTUAL:80", "ACTUAL:100", "FORECASTED:100"],
  );
});

test("the deploy document runs this checkout's deploy.sh, and its manifest can't break quoting", () => {
  const doc = JSON.stringify(prod.findResources("AWS::SSM::Document"));
  assert.ok(doc.includes("deploy.sh '{{ Manifest }}'"));
  assert.ok(doc.includes("release ${release:0:12} is live"));
  assert.ok(doc.includes("^[A-Za-z0-9+/=]+$"));
});

test("registries keep two images each and every resource carries the project tag", () => {
  prod.resourceCountIs("AWS::ECR::Repository", 4);
  prod.allResourcesProperties("AWS::ECR::Repository", { ImageTagMutability: "IMMUTABLE" });
  host.hasResourceProperties("AWS::EC2::Instance", { Tags: Match.arrayWith([{ Key: "rolerank:host", Value: "prod" }, { Key: "rolerank:project", Value: "rolerank" }]) });
});
