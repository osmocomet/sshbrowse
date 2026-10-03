import assert from "node:assert/strict";
import test from "node:test";
import type { Connection } from "../bindings/sshbrowse/internal/profile/models";
import { connectionDialogBounds, connectionFormSections, forwardingSummary, portForwardingSummary, revealInvalidConnectionField } from "../src/lib/connectionForm.ts";

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

test("forwarding summaries identify enabled options without claiming SSH defaults are off", () => {
  assert.equal(forwardingSummary(basic), "Use your local SSH agent or X11 display remotely");
  assert.equal(forwardingSummary({ ...basic, agentForwarding: true }), "SSH agent enabled");
  assert.equal(forwardingSummary({ ...basic, x11Forwarding: true }), "X11 enabled");
  assert.equal(forwardingSummary({ ...basic, agentForwarding: true, x11Forwarding: true }), "SSH agent and X11 enabled");
});

test("tunnel summaries count nonblank lines across all forwarding types", () => {
  assert.equal(portForwardingSummary("", " \n", ""), "Local, remote, and dynamic tunnels");
  assert.equal(portForwardingSummary("8080:localhost:80\n", "", ""), "1 tunnel configured");
  assert.equal(portForwardingSummary("8080:localhost:80\n\n8081:localhost:81", "9090:localhost:90", "1080\n"), "4 tunnels configured");
});


test("dialog bounds retain screen insets below and above 100% scale and on resize", () => {
  for (const [width, height] of [[1280, 900], [640, 480]]) {
    for (const scale of [0.7, 1, 1.5]) {
      const bounds = connectionDialogBounds(width, height, scale);
      assert.equal(bounds.width * scale, width - 32);
      assert.equal(bounds.height * scale, height - 32);
      assert.equal(bounds.top * scale, 16);
    }
  }
});

function validationForm({ emptySavedJump = false, invalid = true, nested = false } = {}) {
  const calls: string[] = [];
  const outer = { open: false, parentElement: null };
  const disclosure = { open: false, parentElement: nested ? { closest: () => outer } : null };
  const field = {
    value: emptySavedJump ? "" : "bad",
    validationMessage: "Invalid field value.",
    closest: () => disclosure,
    focus: () => {
      assert.equal(disclosure.open, true);
      if (nested) assert.equal(outer.open, true);
      calls.push("focus");
    },
    reportValidity: () => { calls.push("report"); return false; },
  };
  const form = {
    querySelectorAll: (selector: string) => {
      assert.equal(selector, "select[required]:not(:disabled)");
      return emptySavedJump ? [field] : [];
    },
    querySelector: (selector: string) => {
      assert.equal(selector, ":invalid");
      return invalid ? field : null;
    },
  } as unknown as HTMLFormElement;
  return { form, calls, disclosure };
}

test("invalid hidden fields open their disclosures before focus and validation reporting", () => {
  const { form, calls } = validationForm({ nested: true });
  assert.equal(revealInvalidConnectionField(form), "Invalid field value.");
  assert.deepEqual(calls, ["focus", "report"]);
});

test("an empty saved jump selection reports an actionable error even if native validity misses it", () => {
  const { form, calls } = validationForm({ emptySavedJump: true, invalid: false });
  assert.equal(revealInvalidConnectionField(form), "Choose a saved jump connection.");
  assert.deepEqual(calls, ["focus", "report"]);
});

test("a valid form leaves disclosures and focus unchanged", () => {
  const { form, calls, disclosure } = validationForm({ invalid: false });
  assert.equal(revealInvalidConnectionField(form), null);
  assert.equal(disclosure.open, false);
  assert.deepEqual(calls, []);
});
