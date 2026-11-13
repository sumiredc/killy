// Wire shapes of the Killy server's App API (docs/openapi.yaml is their source of truth).

/** A server as this phone sees it (Server). */
export type Server = {
  id: string;
  name: string;
  hostname: string;
  notify: boolean;
};

/** A parent with zombies on a server, with its first-detection time (ServerState.parents). */
export type ParentState = {
  pid: number;
  starttime: number;
  cmd: string;
  zombies: number[];
  first_detected_at: string;
};

/** A linked server and its latest parents (ServerState). */
export type ServerState = Server & {
  disconnected: boolean;
  parents: ParentState[];
};

/** Data of the `state` event; replaces what the phone had (State). */
export type State = {
  servers: ServerState[];
};
