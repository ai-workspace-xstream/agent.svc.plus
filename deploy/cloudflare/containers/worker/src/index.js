import { Container } from "@cloudflare/containers";
import { env as workerEnv } from "cloudflare:workers";

export class AgentRuntimeContainer extends Container {
  defaultPort = 8080;
  sleepAfter = "10m";
  envVars = {
    HEALTH_PORT: "8080",
    AGENT_ID: workerEnv.AGENT_ID ?? "",
    AGENT_CONTROLLER_URL: workerEnv.AGENT_CONTROLLER_URL ?? "",
    AGENT_API_TOKEN: workerEnv.AGENT_API_TOKEN ?? "",
    DOMAIN: workerEnv.DOMAIN ?? "",
    CONTROLLER_URL: workerEnv.CONTROLLER_URL ?? "",
    INTERNAL_SERVICE_TOKEN: workerEnv.INTERNAL_SERVICE_TOKEN ?? "",
  };
}

function json(payload, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
    },
  });
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const nodeID = url.searchParams.get("node") || "default";
    const container = env.AGENT_RUNTIME.getByName(nodeID);

    if (url.pathname === "/healthz") {
      return json({
        ok: true,
        service: "agent-svc-plus-container-control-plane",
      });
    }

    if (url.pathname === "/container/healthz") {
      return container.fetch("http://container/healthz");
    }

    if (url.pathname === "/container/readyz") {
      return container.fetch("http://container/readyz");
    }

    if (url.pathname === "/container/debug/processes") {
      return container.fetch("http://container/debug/processes");
    }

    return json(
      {
        error: "not_found",
        message:
          "use /container/healthz, /container/readyz or /container/debug/processes",
      },
      404,
    );
  },
};
