import assert from "node:assert/strict";
import test from "node:test";
import type { Connection } from "../bindings/sshbrowse/internal/profile/models";
import { connectionFormSections } from "../src/lib/connectionForm.ts";

const basic: Connection = {
  id: "", name: "", folder: "", host: "example.invalid", user: "", port: 0,
  identityFile: "", jumpHost: "", agentForwarding: false, x11Forwarding: false,
  localForwards: [], remoteForwards: [], dynamicForwards: [], provenance: null,
};

test("basic and legacy connections keep optional sections collapsed", () => {
  assert.deepEqual(connectionFormSections(basic), {
    authentication: false, routing: false, forwarding: false, tunnels: false,
  });
  assert.deepEqual(connectionFormSections({ ...basic, localForwards: null, remoteForwards: null, dynamicForwards: null }),
    connectionFormSections(basic));
});

test("saved authentication and either jump-host format open their own sections", () => {
  assert.deepEqual(connectionFormSections({ ...basic, identityFile: "~/.ssh/example" }), {
    authentication: true, routing: false, forwarding: false, tunnels: false,
  });
  for (const routing of [{ jumpHost: "jump.invalid" }, { jumpConnectionId: "saved-jump" }]) {
    assert.deepEqual(connectionFormSections({ ...basic, ...routing }), {
      authentication: false, routing: true, forwarding: false, tunnels: false,
    });
  }
});

test("agent and X11 forwarding open independently of port tunnels", () => {
  for (const forwarding of [{ agentForwarding: true }, { x11Forwarding: true }]) {
    assert.deepEqual(connectionFormSections({ ...basic, ...forwarding }), {
      authentication: false, routing: false, forwarding: true, tunnels: false,
    });
  }
});

test("each saved tunnel type opens Port forwarding without opening agent/X11 options", () => {
  for (const tunnels of [
    { localForwards: ["8080:localhost:80"] },
    { remoteForwards: ["9090:localhost:90"] }, { dynamicForwards: ["1080"] },
  ]) {
    assert.deepEqual(connectionFormSections({ ...basic, ...tunnels }), {
      authentication: false, routing: false, forwarding: false, tunnels: true,
    });
  }
});
