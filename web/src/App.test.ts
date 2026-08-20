import { describe, expect, it } from "vitest";
import { parseRoute } from "./App";

describe("parseRoute", () => {
  it("parses a table URL", () => {
    expect(parseRoute("/channels/channel-1/tables/table-2")).toEqual({
      page: "channel",
      channelID: "channel-1",
      tableID: "table-2",
    });
  });

  it("parses the personal settings URL", () => {
    expect(parseRoute("/settings")).toEqual({ page: "settings" });
  });

  it("rejects malformed URI encoding without throwing", () => {
    expect(parseRoute("/channels/%E0%A4%A")).toEqual({ page: "not_found" });
  });
});
