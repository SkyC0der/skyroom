"use client";

import { useEffect, useState } from "react";

type Status = "checking" | "available" | "unavailable";

const apiBaseURL =
  process.env.NEXT_PUBLIC_API_BASE_URL ??
  (process.env.NODE_ENV === "development" ? "http://localhost:8080" : "");

export function ServiceStatus() {
  const [status, setStatus] = useState<Status>("checking");

  useEffect(() => {
    const controller = new AbortController();

    fetch(`${apiBaseURL}/api/v1/health`, {
      cache: "no-store",
      signal: controller.signal,
    })
      .then((response) => {
        if (!response.ok) throw new Error("health check failed");
        setStatus("available");
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "AbortError") return;
        setStatus("unavailable");
      });

    return () => controller.abort();
  }, []);

  return (
    <p className={`service-status service-status--${status}`} aria-live="polite">
      <span aria-hidden="true" />
      {status === "checking" && "Checking the room"}
      {status === "available" && "Service available"}
      {status === "unavailable" && "Service starting soon"}
    </p>
  );
}
