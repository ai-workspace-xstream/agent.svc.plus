const PROXYABLE_PATHS = new Set([
  "/api/agent-server/v1/users",
  "/api/agent-server/v1/status",
]);

function json(payload, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
    },
  });
}

function getBearerToken(headerValue) {
  if (!headerValue) {
    return "";
  }
  const value = headerValue.trim();
  if (!value.toLowerCase().startsWith("bearer ")) {
    return "";
  }
  return value.slice(7).trim();
}

function readInboundToken(request) {
  return (
    getBearerToken(request.headers.get("authorization")) ||
    request.headers.get("x-service-token")?.trim() ||
    ""
  );
}

function withTrailingSlash(url) {
  if (url.pathname.endsWith("/")) {
    return url;
  }
  url.pathname += "/";
  return url;
}

async function forward(request, env, upstreamURL) {
  const headers = new Headers(request.headers);
  headers.delete("host");
  headers.set("x-forwarded-host", new URL(request.url).host);
  headers.set("x-forwarded-proto", "https");

  const outboundToken = env.FORCE_SERVICE_TOKEN?.trim();
  if (outboundToken) {
    headers.set("authorization", `Bearer ${outboundToken}`);
    headers.set("x-service-token", outboundToken);
  }

  const method = request.method.toUpperCase();
  const init = {
    method,
    headers,
    redirect: "follow",
  };
  if (method !== "GET" && method !== "HEAD") {
    init.body = request.body;
  }

  const upstreamResp = await fetch(upstreamURL.toString(), init);
  const respHeaders = new Headers(upstreamResp.headers);
  respHeaders.set("x-agent-edge", "cloudflare-worker");
  return new Response(upstreamResp.body, {
    status: upstreamResp.status,
    statusText: upstreamResp.statusText,
    headers: respHeaders,
  });
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname === "/healthz") {
      return json({
        ok: true,
        service: "agent-svc-plus-edge",
        now: new Date().toISOString(),
      });
    }

    if (!PROXYABLE_PATHS.has(url.pathname)) {
      return json(
        {
          error: "not_found",
          message: "only agent users/status endpoints are exposed",
        },
        404,
      );
    }

    const base = env.CONTROLLER_BASE_URL?.trim();
    if (!base) {
      return json(
        {
          error: "misconfigured",
          message: "missing CONTROLLER_BASE_URL",
        },
        500,
      );
    }

    const requiredInboundToken = env.EDGE_ACCESS_TOKEN?.trim();
    if (requiredInboundToken) {
      const inboundToken = readInboundToken(request);
      if (inboundToken !== requiredInboundToken) {
        return json(
          {
            error: "unauthorized",
            message: "invalid edge access token",
          },
          401,
        );
      }
    }

    let upstreamBaseURL;
    try {
      upstreamBaseURL = withTrailingSlash(new URL(base));
    } catch {
      return json(
        {
          error: "misconfigured",
          message: "CONTROLLER_BASE_URL must be a valid URL",
        },
        500,
      );
    }

    const upstreamURL = new URL(url.pathname + url.search, upstreamBaseURL);
    try {
      return await forward(request, env, upstreamURL);
    } catch (err) {
      return json(
        {
          error: "upstream_unavailable",
          message: err instanceof Error ? err.message : "request failed",
        },
        502,
      );
    }
  },
};
