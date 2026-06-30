import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { viteSingleFile } from "vite-plugin-singlefile";
import fs from "fs";
import path from "path";

const pkgPath = path.resolve(__dirname, "../npm/package.json");
const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf-8"));

export default defineConfig({
  plugins: [svelte(), viteSingleFile()],
  root: "./src/ui",
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version),
  },
  build: {
    target: "es2015",
    cssCodeSplit: false,
    outDir: "../../dist",
    rollupOptions: {
      output: {
        inlineDynamicImports: true,
      },
    },
    emptyOutDir: true,
  },
});
