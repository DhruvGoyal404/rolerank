import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";

import { CfnOutput, Duration, RemovalPolicy, Stack, Tags, type StackProps } from "aws-cdk-lib";
import * as budgets from "aws-cdk-lib/aws-budgets";
import * as cloudwatch from "aws-cdk-lib/aws-cloudwatch";
import * as cwActions from "aws-cdk-lib/aws-cloudwatch-actions";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as ecr from "aws-cdk-lib/aws-ecr";
import * as iam from "aws-cdk-lib/aws-iam";
import * as s3 from "aws-cdk-lib/aws-s3";
import * as sns from "aws-cdk-lib/aws-sns";
import * as snsSubs from "aws-cdk-lib/aws-sns-subscriptions";
import * as ssm from "aws-cdk-lib/aws-ssm";
import type { Construct } from "constructs";

// Two stacks (docs/ARCHITECTURE.md, "Deployment design"), after CodeJudge's infra/:
// RoleRankProd stays while the site is down and costs ~nothing idle; RoleRankHost holds
// everything that bills while idle and is destroyed by rolerank-down.sh.

/** A host script as LF text: a Windows checkout may have CRLF, the host must not. */
const hostScript = (path: string) => readFileSync(new URL(`../../deploy/${path}`, import.meta.url), "utf8").replace(/\r\n/g, "\n");

export const REGION = "ap-south-1";
/**
 * The OIDC subject GitHub issues for main in this repository. Immutable subjects are on
 * (gh api repos/DhruvGoyal404/rolerank/actions/oidc/customization/sub, 2026-11-09), so the
 * owner's and the repository's numeric IDs are part of it: a re-created repository of the same
 * name does not match.
 */
export const GITHUB_SUB = "repo:DhruvGoyal404@151380878/rolerank@1440707842:ref:refs/heads/main";
export const PARAM_PREFIX = "/rolerank/prod/";
export const REPOS = ["api", "web", "worker", "sandbox"] as const;
export const DEPLOY_DOCUMENT = "RoleRank-Deploy";
export const BACKUP_PREFIX = "rolerank/pg/";
export const FINAL_PREFIX = "rolerank/final/";
export const METRIC_NAMESPACE = "RoleRank";
export const HOST_TAG = { key: "rolerank:host", value: "prod" } as const;
/** Every resource carries it; the budget counts only costs with this tag. */
export const PROJECT_TAG = { key: "rolerank:project", value: "rolerank" } as const;
/**
 * Monthly cost alert in USD, the account's billing currency: Rs 500 at Rs 94.95 to the dollar
 * (ARCHITECTURE.md, "Cost"). An alert, not a cap: AWS keeps billing past it.
 */
export const BUDGET_USD = 5.27;
export const HOST_INSTANCE = "m7g.xlarge";
/**
 * Arm64 types the host may run on, in fallback order (rolerank-up.sh tries every AZ for one type
 * before the next): 4 vCPU each, 16 GiB or more. Graviton, so the arm64 images and the gVisor
 * build are the same on all of them.
 */
export const HOST_TYPES = [HOST_INSTANCE, "m6g.xlarge", "r7g.xlarge"] as const;
const HOST_AZ = /^ap-south-1[abc]$/;
export const HOST_DISK_GB = 40;
export const AMI_ID = /^ami-[0-9a-f]{8,17}$/;
/** Longest base64 release manifest the deploy document accepts (one is ~1 KB). */
export const MANIFEST_MAX_CHARS = 4000;

export interface ProdStackProps extends StackProps {
  alarmEmail: string;
  budgetEmail?: string;
}

export class RoleRankProdStack extends Stack {
  readonly backups: s3.Bucket;
  readonly topic: sns.Topic;
  readonly repos: ecr.Repository[];

  constructor(scope: Construct, id: string, props: ProdStackProps) {
    super(scope, id, props);

    // --- Backups ----------------------------------------------------------------
    // Object Lock (governance, 14 days): no one, the host included, deletes a backup within
    // retention. The host may only put and get under rolerank/pg/; rolerank-down.sh keeps the
    // finals under rolerank/final/, pruned to the newest few once out of retention.
    const backups = new s3.Bucket(this, "Backups", {
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
      enforceSSL: true,
      versioned: true,
      objectLockEnabled: true,
      objectLockDefaultRetention: s3.ObjectLockRetention.governance(Duration.days(14)),
      objectOwnership: s3.ObjectOwnership.BUCKET_OWNER_ENFORCED,
      lifecycleRules: [
        { id: "nightly-14-days", prefix: BACKUP_PREFIX, expiration: Duration.days(14), abortIncompleteMultipartUploadAfter: Duration.days(1) },
        { id: "old-versions", noncurrentVersionExpiration: Duration.days(15), expiredObjectDeleteMarker: true },
      ],
      removalPolicy: RemovalPolicy.RETAIN,
    });
    this.backups = backups;
    new ssm.StringParameter(this, "BackupBucketParam", {
      parameterName: `${PARAM_PREFIX}backup_bucket`,
      stringValue: backups.bucketName,
      description: "S3 bucket for RoleRank backups (deploy.sh writes it into backup.env)",
    });

    // --- Alarms ---------------------------------------------------------------------
    const topic = new sns.Topic(this, "Alarms", { displayName: "RoleRank alarms" });
    this.topic = topic;
    topic.addSubscription(new snsSubs.EmailSubscription(props.alarmEmail));
    // No successful backup in 26 hours: 26 hourly sums all below 1 (missing counts as
    // breaching). rolerank-down.sh disables its actions while the host is gone; up re-enables
    // them after a fresh backup.
    const backupMissing = new cloudwatch.Alarm(this, "BackupMissing", {
      alarmDescription: "No successful RoleRank backup in 26 hours (RoleRank/BackupSucceeded)",
      metric: new cloudwatch.Metric({ namespace: METRIC_NAMESPACE, metricName: "BackupSucceeded", statistic: "Sum", period: Duration.hours(1) }),
      threshold: 1,
      comparisonOperator: cloudwatch.ComparisonOperator.LESS_THAN_THRESHOLD,
      evaluationPeriods: 26,
      datapointsToAlarm: 26,
      treatMissingData: cloudwatch.TreatMissingData.BREACHING,
    });
    backupMissing.addAlarmAction(new cwActions.SnsAction(topic));

    // --- Budgets: alerts, not caps -----------------------------------------------------------
    // The project budget is filtered to this project's cost-allocation tag, which counts nothing
    // until the tag is active in Billing: the account also holds CodeJudge (QUESTIONS.md, Q15).
    // The safety budget is account-wide, so it covers that gap (and CodeJudge's spend too).
    const address = props.budgetEmail ?? props.alarmEmail;
    const email = [{ subscriptionType: "EMAIL", address }];
    // A subscriber change replaces a budget, and a replacement can't reuse the old name.
    const suffix = createHash("sha256").update(address).digest("hex").slice(0, 8);
    const notifications = [
      ...[50, 80, 100].map((threshold) => ({
        notification: { notificationType: "ACTUAL", comparisonOperator: "GREATER_THAN", threshold, thresholdType: "PERCENTAGE" },
        subscribers: email,
      })),
      {
        notification: { notificationType: "FORECASTED", comparisonOperator: "GREATER_THAN", threshold: 100, thresholdType: "PERCENTAGE" },
        subscribers: email,
      },
    ];
    const budget = { budgetType: "COST", timeUnit: "MONTHLY", budgetLimit: { amount: BUDGET_USD, unit: "USD" } };
    new budgets.CfnBudget(this, "MonthlyBudget", {
      budget: { ...budget, budgetName: `rolerank-monthly-${suffix}`, costFilters: { TagKeyValue: [`user:${PROJECT_TAG.key}$${PROJECT_TAG.value}`] } },
      notificationsWithSubscribers: notifications,
    });
    new budgets.CfnBudget(this, "AccountBudget", {
      budget: { ...budget, budgetName: `rolerank-account-safety-${suffix}` },
      notificationsWithSubscribers: notifications,
    });

    // --- Registries: the current and the previous release ------------------------------------
    // Kept while down, so up redeploys the saved release by digest without a build (~$0.10/GB-month).
    this.repos = REPOS.map(
      (name) =>
        new ecr.Repository(this, `Repo-${name}`, {
          repositoryName: `rolerank/${name}`,
          imageScanOnPush: true,
          imageTagMutability: ecr.TagMutability.IMMUTABLE,
          encryption: ecr.RepositoryEncryption.AES_256,
          lifecycleRules: [{ description: "keep the current and the previous release", maxImageCount: 2 }],
          removalPolicy: RemovalPolicy.RETAIN,
        }),
    );

    // --- Deploy document ----------------------------------------------------------------------
    const deployScript = hostScript("host/deploy.sh");
    const doc = new ssm.CfnDocument(this, "DeployDocument", {
      name: DEPLOY_DOCUMENT,
      documentType: "Command",
      documentFormat: "JSON",
      updateMethod: "NewVersion",
      content: {
        schemaVersion: "2.2",
        description: "Deploy a RoleRank release (images pinned by digest); rolls back on failure.",
        parameters: {
          // The base64 alphabet only: pasted into a single-quoted shell word, it can't break out.
          Manifest: { type: "String", description: "base64 of release.json", allowedPattern: "^[A-Za-z0-9+/=]+$", maxChars: MANIFEST_MAX_CHARS },
        },
        mainSteps: [
          {
            action: "aws:runShellScript",
            name: "deploy",
            inputs: {
              timeoutSeconds: "1800",
              runCommand: [
                "set -eu",
                "install -d -m 0755 /opt/rolerank/bin",
                `cat > /opt/rolerank/bin/deploy.sh.new <<'ROLERANK_DEPLOY'\n${deployScript}ROLERANK_DEPLOY`,
                "chmod 0755 /opt/rolerank/bin/deploy.sh.new",
                "mv -f /opt/rolerank/bin/deploy.sh.new /opt/rolerank/bin/deploy.sh",
                "exec /bin/bash /opt/rolerank/bin/deploy.sh '{{ Manifest }}'",
              ],
            },
          },
        ],
      },
    });

    // --- GitHub OIDC deploy role -----------------------------------------------------------
    // The account's GitHub OIDC provider exists already (CodeJudgeProd made it; there is one
    // per URL per account), so it is referenced, not created.
    const providerArn = `arn:${this.partition}:iam::${this.account}:oidc-provider/token.actions.githubusercontent.com`;
    const deployRole = new iam.Role(this, "DeployRole", {
      roleName: "rolerank-github-deploy",
      description: "GitHub Actions on DhruvGoyal404/rolerank main: push images, run the deploy document",
      maxSessionDuration: Duration.hours(2),
      assumedBy: new iam.WebIdentityPrincipal(providerArn, {
        StringEquals: { "token.actions.githubusercontent.com:aud": "sts.amazonaws.com", "token.actions.githubusercontent.com:sub": GITHUB_SUB },
      }),
    });
    deployRole.addToPolicy(
      new iam.PolicyStatement({
        sid: "PushOwnImages",
        actions: ["ecr:BatchCheckLayerAvailability", "ecr:BatchGetImage", "ecr:CompleteLayerUpload", "ecr:DescribeImages", "ecr:InitiateLayerUpload", "ecr:PutImage", "ecr:UploadLayerPart"],
        resources: this.repos.map((r) => r.repositoryArn),
      }),
    );
    deployRole.addToPolicy(new iam.PolicyStatement({ sid: "RegistryLogin", actions: ["ecr:GetAuthorizationToken"], resources: ["*"] }));
    deployRole.addToPolicy(
      new iam.PolicyStatement({ sid: "RunDeployDocument", actions: ["ssm:SendCommand"], resources: [this.formatArn({ service: "ssm", resource: "document", resourceName: doc.ref })] }),
    );
    deployRole.addToPolicy(
      new iam.PolicyStatement({
        sid: "RunOnTaggedHost",
        actions: ["ssm:SendCommand"],
        resources: [this.formatArn({ service: "ec2", resource: "instance", resourceName: "*" })],
        conditions: { StringEquals: { [`ssm:resourceTag/${HOST_TAG.key}`]: HOST_TAG.value } },
      }),
    );
    deployRole.addToPolicy(
      new iam.PolicyStatement({ sid: "FindHostAndOutcome", actions: ["ec2:DescribeInstances", "ssm:GetCommandInvocation", "ssm:ListCommandInvocations"], resources: ["*"] }),
    );

    new CfnOutput(this, "BackupBucket", { value: backups.bucketName });
    new CfnOutput(this, "Registry", { value: `${this.account}.dkr.ecr.${this.region}.amazonaws.com` });
    new CfnOutput(this, "DeployRoleArn", { value: deployRole.roleArn });
    new CfnOutput(this, "AlarmTopicArn", { value: topic.topicArn });
  }
}

export interface HostStackProps extends StackProps {
  persistent: RoleRankProdStack;
  /** Synthesise without credentials (CI, tests): placeholders stand in for lookups. Never deploy offline. */
  offline?: boolean;
  /**
   * Ubuntu 24.04 arm64, pinned (cdk.json "rolerank:hostAmi"). A literal ID keeps the instance's
   * ImageId fixed; resolving "current" at deploy time would replace the host on every Ubuntu
   * release. OS updates happen on the host (unattended-upgrades).
   */
  hostAmi: string;
  /** The AZ to place the host in (rolerank-up.sh chooses it per attempt); the default VPC's first otherwise. */
  hostAz?: string | undefined;
  /** One of HOST_TYPES; HOST_INSTANCE otherwise. */
  hostType?: string | undefined;
}

export class RoleRankHostStack extends Stack {
  constructor(scope: Construct, id: string, props: HostStackProps) {
    super(scope, id, props);
    const ami = props.hostAmi || (props.offline ? "ami-00000000000000000" : "");
    if (!AMI_ID.test(ami)) throw new Error(`pin the host AMI in infra/cdk.json "rolerank:hostAmi" (Ubuntu 24.04 arm64), got ${JSON.stringify(props.hostAmi)}`);
    const type = props.hostType ?? HOST_INSTANCE;
    if (!(HOST_TYPES as readonly string[]).includes(type)) throw new Error(`host type ${type} is not one of ${HOST_TYPES.join(", ")}`);
    if (props.hostAz !== undefined && !HOST_AZ.test(props.hostAz)) throw new Error(`host AZ ${props.hostAz} is not in ${REGION}`);

    const vpc = props.offline
      ? ec2.Vpc.fromVpcAttributes(this, "DefaultVpc", {
          vpcId: "vpc-00000000000000000",
          availabilityZones: [`${REGION}a`, `${REGION}b`, `${REGION}c`],
          publicSubnetIds: ["subnet-0000000000000000a", "subnet-0000000000000000b", "subnet-0000000000000000c"],
          publicSubnetRouteTableIds: ["rtb-00000000000000000", "rtb-00000000000000000", "rtb-00000000000000000"],
        })
      : ec2.Vpc.fromLookup(this, "DefaultVpc", { isDefault: true });
    const backups = props.persistent.backups;

    // --- Instance role: least privilege ------------------------------------------------
    const role = new iam.Role(this, "InstanceRole", {
      assumedBy: new iam.ServicePrincipal("ec2.amazonaws.com"),
      description: "RoleRank host: SSM agent, own parameters, backup prefix (put and get only), image pulls",
      managedPolicies: [iam.ManagedPolicy.fromAwsManagedPolicyName("AmazonSSMManagedInstanceCore")],
    });
    const paramArn = this.formatArn({ service: "ssm", resource: "parameter", resourceName: PARAM_PREFIX.slice(1) + "*" });
    role.addToPolicy(
      new iam.PolicyStatement({
        sid: "ReadOwnParameters",
        actions: ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"],
        resources: [paramArn, this.formatArn({ service: "ssm", resource: "parameter", resourceName: PARAM_PREFIX.slice(1, -1) })],
      }),
    );
    role.addToPolicy(
      new iam.PolicyStatement({
        sid: "DecryptOwnParameters",
        actions: ["kms:Decrypt"],
        resources: [this.formatArn({ service: "kms", resource: "key", resourceName: "*" })],
        conditions: { StringEquals: { "kms:ViaService": `ssm.${this.region}.amazonaws.com` }, StringLike: { "kms:EncryptionContext:PARAMETER_ARN": paramArn } },
      }),
    );
    role.addToPolicy(new iam.PolicyStatement({ sid: "BackupPrefixObjects", actions: ["s3:PutObject", "s3:GetObject"], resources: [backups.arnForObjects(`${BACKUP_PREFIX}*`)] }));
    role.addToPolicy(
      new iam.PolicyStatement({
        sid: "BackupPrefixList",
        actions: ["s3:ListBucket"],
        resources: [backups.bucketArn],
        conditions: { StringLike: { "s3:prefix": [`${BACKUP_PREFIX}*`] } },
      }),
    );
    role.addToPolicy(
      new iam.PolicyStatement({
        sid: "PullOwnImages",
        actions: ["ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer", "ecr:BatchCheckLayerAvailability"],
        resources: props.persistent.repos.map((r) => r.repositoryArn),
      }),
    );
    role.addToPolicy(new iam.PolicyStatement({ sid: "RegistryLogin", actions: ["ecr:GetAuthorizationToken"], resources: ["*"] }));
    role.addToPolicy(
      new iam.PolicyStatement({
        sid: "HostMetrics",
        actions: ["cloudwatch:PutMetricData"],
        resources: ["*"],
        conditions: { StringEquals: { "cloudwatch:namespace": METRIC_NAMESPACE } },
      }),
    );

    // --- Host ------------------------------------------------------------------------------
    const sg = new ec2.SecurityGroup(this, "HostSg", { vpc, description: "RoleRank host: HTTP and HTTPS only (no SSH; access via SSM)", allowAllOutbound: true });
    for (const peer of [ec2.Peer.anyIpv4(), ec2.Peer.anyIpv6()]) {
      sg.addIngressRule(peer, ec2.Port.tcp(80), "HTTP (redirects to HTTPS, ACME)");
      sg.addIngressRule(peer, ec2.Port.tcp(443), "HTTPS");
      sg.addIngressRule(peer, ec2.Port.udp(443), "HTTP/3");
    }
    // No secrets in user data: the bootstrap only installs software; deploy.sh reads SSM.
    const userData = ec2.UserData.forLinux();
    userData.addCommands(
      "set -euo pipefail",
      "install -d -m 0755 /opt/rolerank/bootstrap",
      `cat > /opt/rolerank/bootstrap/bootstrap.sh <<'ROLERANK_BOOTSTRAP'\n${hostScript("host/bootstrap.sh")}ROLERANK_BOOTSTRAP`,
      "chmod 0755 /opt/rolerank/bootstrap/bootstrap.sh",
      "/opt/rolerank/bootstrap/bootstrap.sh",
    );
    const host = new ec2.Instance(this, "Host", {
      vpc,
      // Any AZ works: the Elastic IP, the backups and the Caddy data are regional, and the disk is
      // new on every up.
      vpcSubnets: { subnetType: ec2.SubnetType.PUBLIC, ...(props.hostAz ? { availabilityZones: [props.hostAz] } : {}) },
      instanceType: new ec2.InstanceType(type),
      machineImage: ec2.MachineImage.genericLinux({ [REGION]: ami }),
      role,
      securityGroup: sg,
      requireImdsv2: true,
      blockDevices: [
        { deviceName: "/dev/sda1", volume: ec2.BlockDeviceVolume.ebs(HOST_DISK_GB, { volumeType: ec2.EbsDeviceVolumeType.GP3, encrypted: true, deleteOnTermination: true }) },
      ],
      userData,
      userDataCausesReplacement: false,
      detailedMonitoring: false,
    });
    Tags.of(host).add(HOST_TAG.key, HOST_TAG.value);
    const eip = new ec2.CfnEIP(this, "Eip", { domain: "vpc", tags: [{ key: "Name", value: "rolerank" }] });
    new ec2.CfnEIPAssociation(this, "EipAssociation", { allocationId: eip.attrAllocationId, instanceId: host.instanceId });

    // Recover onto healthy hardware if the underlying host fails (same ID, IP and volume).
    const recover = new cloudwatch.Alarm(this, "SystemStatusRecover", {
      alarmDescription: "Underlying hardware failed: recover the instance",
      metric: new cloudwatch.Metric({
        namespace: "AWS/EC2",
        metricName: "StatusCheckFailed_System",
        dimensionsMap: { InstanceId: host.instanceId },
        statistic: "Maximum",
        period: Duration.minutes(1),
      }),
      threshold: 1,
      comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
      evaluationPeriods: 2,
      treatMissingData: cloudwatch.TreatMissingData.MISSING,
    });
    recover.addAlarmAction(new cwActions.Ec2Action(cwActions.Ec2InstanceAction.RECOVER));
    recover.addAlarmAction(new cwActions.SnsAction(props.persistent.topic));

    new CfnOutput(this, "InstanceId", { value: host.instanceId });
    new CfnOutput(this, "PublicIp", { value: eip.ref, description: "the DNS A record points here while up" });
  }
}

/** Tags every resource of both stacks, for the budget's filter and the audit. */
export function tagProject(scope: Construct): void {
  Tags.of(scope).add(PROJECT_TAG.key, PROJECT_TAG.value);
}
