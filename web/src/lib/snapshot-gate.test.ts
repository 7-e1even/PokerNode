import { describe, expect, it } from "vitest";
import { createSnapshotGate } from "./snapshot-gate";

describe("snapshot gate", () => {
  it("rejects an HTTP snapshot after a realtime push", () => {
    const gate = createSnapshotGate();
    const read = gate.beginRead();

    gate.acceptPush();

    expect(gate.acceptRead(read)).toBe(false);
  });

  it("invalidates an older read when a mutation begins", () => {
    const gate = createSnapshotGate();
    const read = gate.beginRead();
    const mutation = gate.beginMutation();

    expect(gate.acceptRead(read)).toBe(false);
    expect(gate.finishMutation(mutation)).toBe(true);
  });

  it("keeps a realtime push when it arrives before a mutation response", () => {
    const gate = createSnapshotGate();
    const mutation = gate.beginMutation();

    gate.acceptPush();

    expect(gate.finishMutation(mutation)).toBe(false);
    expect(gate.beginRead()).not.toBeNull();
  });

  it("pauses reads while a mutation is in flight", () => {
    const gate = createSnapshotGate();
    const mutation = gate.beginMutation();

    expect(gate.beginRead()).toBeNull();
    gate.cancelMutation(mutation);
    expect(gate.beginRead()).not.toBeNull();
  });
});
