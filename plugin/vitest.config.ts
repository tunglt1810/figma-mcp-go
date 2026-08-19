import { defineConfig } from "vitest/config";

// Deliberately separate from vite.config.ts. That config sets root to ./src/ui so
// the UI builds into a single inlined HTML file, but a root of src/ui hides every
// test outside that folder — 12 of the 13 test files. Test discovery should not
// depend on remembering to pass --dir src on the command line.
export default defineConfig({
  test: {
    include: ["src/**/*.test.ts"],
  },
});
