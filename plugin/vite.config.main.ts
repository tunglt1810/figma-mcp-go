import { defineConfig } from "vite";
import fs from "fs";
import path from "path";

const pkgPath = path.resolve(__dirname, "../npm/package.json");
const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf-8"));

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version),
  },
  build: {
    target: "es2015",
    lib: {
      entry: "src/main.ts",
      formats: ["iife"],
      name: "code",
      fileName: () => "code.js",
    },
    outDir: "dist",
    emptyOutDir: false,
    minify: false,
  },
});
