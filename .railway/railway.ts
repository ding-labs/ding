import { defineRailway, github, preserve, project, service, volume } from "railway/iac";

// Every merge to main deploys automatically; this service never pins a commit.
export default defineRailway(() => {
  const data = volume("ding-data", { region: "us-west2", sizeMB: 1024 });
  const app = service("ding-cloud", {
    source: github("ding-labs/ding", { branch: "main" }),
    build: { builder: "DOCKERFILE", dockerfilePath: "Dockerfile.railway" },
    deploy: {
      healthcheckPath: "/readyz", healthcheckTimeout: 120,
      sleepApplication: false, preDeployCommand: [],
      // Losing the PostgreSQL ownership session can stop Ding with exit code 0.
      restartPolicyType: "ALWAYS", restartPolicyMaxRetries: 3,
      multiRegionConfig: { "us-west2": { numReplicas: 1 } },
      overlapSeconds: 0, drainingSeconds: 60,
      limitOverride: { containers: { cpu: 2, memoryBytes: 2147483648 } },
    },
    volumeMounts: { "/data": data },
    domains: [{ domain: "cloud.ding.ing", port: 8080 }],
    env: {
      PORT: "8080", DING_PUBLIC_URL: "https://cloud.ding.ing", DING_NAME: "Ding Cloud",
      // Set these privately before first apply. Preserve operator maintenance
      // mode and restore directory selection across configuration updates.
      DING_MODE: preserve(), DING_STATE_NAME: preserve(),
      DING_PUSH_JSON: preserve(), DING_CAPACITY_JSON: preserve(),
      DING_DATA_KEY_BASE64: preserve(),
      DING_DATABASE_JSON: preserve(), DING_DATABASE_URL: preserve(),
      DING_DATABASE_CA_PEM: preserve(),
      DING_OIDC_JSON: preserve(), DING_MCP_JSON: preserve(),
      DING_MCP_BRIDGE_CLIENTS: preserve(), DING_AUTHKIT_JSON: preserve(),
      DING_AUTHKIT_API_KEY: preserve(),
      DING_OPENAI_APPS_CHALLENGE: preserve(),
    },
  });
  return project("Ding Cloud", { resources: [app, data] });
});
