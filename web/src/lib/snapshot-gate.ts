export interface SnapshotGate {
  beginRead: () => number | null;
  acceptRead: (token: number | null) => boolean;
  acceptPush: () => void;
  beginMutation: () => number;
  finishMutation: (token: number) => boolean;
  cancelMutation: (token: number) => void;
}

export function createSnapshotGate(): SnapshotGate {
  let generation = 0;
  let activeMutation: number | null = null;

  return {
    beginRead() {
      return activeMutation === null ? generation : null;
    },
    acceptRead(token) {
      if (token === null || activeMutation !== null || token !== generation) return false;
      generation += 1;
      return true;
    },
    acceptPush() {
      generation += 1;
    },
    beginMutation() {
      generation += 1;
      activeMutation = generation;
      return activeMutation;
    },
    finishMutation(token) {
      if (activeMutation === token) activeMutation = null;
      if (token !== generation) return false;
      generation += 1;
      return true;
    },
    cancelMutation(token) {
      if (activeMutation === token) activeMutation = null;
    },
  };
}
