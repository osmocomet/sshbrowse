import assert from "node:assert/strict";
import test from "node:test";

import { errorMessage } from "../src/lib/errors.ts";

test("runtime errors are shown without transport prefixes", () => {
  assert.equal(errorMessage("RuntimeError: bad port \"99999\""), "bad port \"99999\"");
  assert.equal(errorMessage(new Error("host is required")), "host is required");
});
