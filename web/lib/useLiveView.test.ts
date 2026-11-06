import { describe, expect, it } from "vitest";
import { apiBase } from "./useLiveView";

describe("where the screen reads from", () => {
  it("is its own origin by default", () => {
    expect(apiBase(undefined, "")).toBe("");
  });

  it("can be pointed elsewhere in a build that names no API, for development", () => {
    expect(apiBase(undefined, "?api=http://localhost:8090")).toBe("http://localhost:8090");
  });

  it("cannot be pointed elsewhere by a link, in a build that names its API", () => {
    expect(apiBase("https://feed.example:8443", "?api=https://evil.example")).toBe(
      "https://feed.example:8443",
    );
  });
});
