export const libraryActionKey = {
  pattern: (id: string) => `pattern:${id}`,
  program: (id: string) => `program:${id}`,
  feedback: (id: number) => `feedback:${id}`,
  exportPattern: (id: string) => `export:pattern:${id}`,
  exportProgram: (id: string) => `export:program:${id}`,
  motionStart: "motion:start",
  playerControl: "motion:player-control",
  playerStop: "motion:stop",
  import: "library:import",
  author: "library:author",
  autoDisable: "library:auto-disable",
} as const;

export type LibraryBusyKeys = ReadonlySet<string>;
