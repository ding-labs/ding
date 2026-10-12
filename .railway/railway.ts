import { defineRailway, github, preserve, project, service, volume } from "railway/iac";

// Deliberately require a reviewed immutable revision; pushes must not deploy
// automatically during backups, restores or schema qualification.
export default defineRailway(() => {
  const commitSha = process.env.DING_DEPLOY_COMMIT;
  if (!commitSha || !/^[a-f0-9]{40}$/.test(commitSha)) {
    throw new Error("Set DING_DEPLOY_COMMIT to the exact reviewed Git commit.");
  }
  const data = volume("ding-data", { region: "us-west2", sizeMB: 1024 });
  const app = service("ding-cloud", {
    source: github("ding-labs/ding", { branch: "codex/railway-cloud", commitSha }),
    build: { builder: "DOCKERFILE", dockerfilePath: "Dockerfile.railway" },
    deploy: {
      healthcheckPath: "/readyz", healthcheckTimeout: 120,
      sleepApplication: false, preDeployCommand: [],
      restartPolicyType: "ON_FAILURE", restartPolicyMaxRetries: 3,
      multiRegionConfig: { "us-west2": { numReplicas: 1 } },
      overlapSeconds: 0, drainingSeconds: 60,
      limitOverride: { containers: { cpu: 1, memoryBytes: 1073741824 } },
    },
    volumeMounts: { "/data": data },
    domains: [{ domain: "cloud.ding.ing", port: 8080 }],
    env: {
      PORT: "8080", DING_PUBLIC_URL: "https://cloud.ding.ing", DING_NAME: "Ding Cloud",
      // Set these privately before first apply. Preserve operator maintenance
      // mode and restore directory selection across configuration updates.
      DING_MODE: preserve(), DING_STATE_NAME: preserve(),
      DING_DATA_KEY_BASE64: preserve(),
      DING_OIDC_JSON: preserve(), DING_MCP_JSON: preserve(),
      DING_MCP_BRIDGE_CLIENTS: preserve(), DING_AUTHKIT_JSON: preserve(),
      DING_OPENAI_APPS_CHALLENGE: preserve(),
    },
  });
  return project("Ding Cloud", { resources: [app, data] });
});
