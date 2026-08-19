# Node read fidelity — Comparing the Figma panel with `get_node` output

Date: 2026-08-19
Sample node: `3659:23522` (`Rectangle 244`, COSUN_APP_CLONE file, `Dev handoff` page)

## 1. Comparison

Current output of `get_node("3659:23522")`:

```json
{
  "id": "3659:23522", "name": "Rectangle 244", "type": "RECTANGLE",
  "bounds": { "x": 0, "y": 0, "width": 168, "height": 229 },
  "geometry": { "rotation": 0, "cornerRadius": 16,
    "topLeftRadius": 16, "topRightRadius": 16,
    "bottomLeftRadius": 16, "bottomRightRadius": 16 },
  "styles": { "cornerRadius": 16, "fills": [ { "type": "GRADIENT_RADIAL", … } ] }
}
```

| Panel section | Value | Output | Status |
| --- | --- | --- | --- |
| Position X / Y | 0, 0 | `bounds.x/y` | matches |
| Rotation | 0° | `geometry.rotation` | matches |
| Dimensions W / H | 168, 229 | `bounds.width/height` | matches |
| Appearance → Opacity | 100% | — | **missing** |
| Appearance → blend mode | Normal | — | **missing** |
| Appearance → visible (eye) | on | — | **missing** |
| Appearance → Corner radius | 16 | `geometry` + `styles` | matches but repeated 5 times |
| Fill → type | Radial | `type: GRADIENT_RADIAL` | matches |
| Fill → opacity | 100% | `opacity` | matches (recently added) |
| Fill → visible (eye) | on | — | **missing** |
| Fill → Stops | 40% F8C8DC 100%, 100% FFF3F3 100% | `stops` | matches (float noise recently fixed) |
| Fill → geometry | center 50/50, radius 106/80, rotation 0 | `geometry` | matches |
| Stroke | empty | omitted | matches |
| Effects | empty | — | **never read** |

Radial gradients are recognized completely correctly. The gaps below are at the node layer, not the gradient layer.

## 2. Evidence for Each Gap

A search across the entire read path (`serializers.ts`, `read-document.ts`, `read-handlers.ts`, `read-export.ts`) for `layoutMode|strokeWeight|strokeAlign|.effects|blendMode|.locked|constraints` returns **0 matches**. This is not an isolated bug; it is an entire group of properties that has never been serialized.

Read/write asymmetry:

| Writable | Readable |
| --- | --- |
| `set_node_properties`: visible, locked, opacity, rotation, blendMode, constraints | only rotation |
| `set_effects`: 4 effect types | nothing |
| `set_paint`: strokeWeight | stroke color only |
| `create_node`: layoutMode, itemSpacing, alignment, sizing, wrap | padding only |

The prompt `internal/prompts/design_token_generation_strategy.go` asks the agent to collect `itemSpacing` from FRAME nodes. This is currently impossible because the read path does not return that field.

The detail level is inverted in `read-document.ts:99-101`: `detail: "compact"` includes `opacity` and `visible`, while `detail: "full"` (the default) falls through to `serializeNode` and loses both. The more detailed mode returns less information.

Output noise, measured on the `get_node("3659:23362")` dump (~50 KB, 45 frames + 90 rectangles):
- Each FRAME emits six all-zero `geometry` fields and all-zero `padding` → about 130 useless characters × 45.
- Each RECTANGLE repeats `cornerRadius` five times (once in `styles`, once plus four corner values in `geometry`) → about 90 useless characters × 90.
- GROUP emits an empty `styles: {}`.

Approximately 14 KB of 50 KB is redundant.

## 3. Plan

### Phase P0 — Data Loss (Highest Priority)

The data exists on the node but is not mentioned in the output at all. An agent reading a node with a shadow would assume that the node has no shadow.

1. **Node appearance** — add a `serializeAppearance(node)` helper in `serializers.ts` and call it from `serializeNode`. Emit fields only when they differ from defaults: `opacity` (≠1), `visible: false`, `blendMode` (≠ NORMAL/PASS_THROUGH), and `locked: true`.
   → Verify: a node with `{opacity: 0.5, visible: false}` returns both fields; a default node returns an empty object.
2. **Remove the detail-level inversion** — have `read-document.ts` call `serializeAppearance` instead of the inline block at lines 99–101, so `full` always includes it.
   → Verify: `get_design_context` with `detail: full` and `compact` both includes `opacity`.
3. **Effects** — serialize `serializeEffects(node.effects)` into `styles.effects`, mirroring the parameter shape of `set_effects` so the value can round-trip:
   `{ type, color, radius, offsetX, offsetY, spread, visible? }`.
   Add `styles.effectStyle` (the style name) alongside `fillStyle`/`strokeStyle`. Omit it when the array is empty.
   → Verify: a node with a DROP_SHADOW returns the correct radius/offset/color; `set_effects` accepts that output without transformation.
4. **Stroke geometry** — emit `styles.strokeWeight` and `styles.strokeAlign` when a stroke exists, and `styles.dashPattern` when non-empty.
   → Verify: a 2px INSIDE stroke returns both weight and alignment.

### Phase P1 — Noise (Reduce Tokens on Every Read)

5. `getGeometry`: emit `rotation` only when it is not 0. Emit corner radii only when they are not 0; when all four corners are equal, emit only `cornerRadius` and omit the four child fields.
6. Remove `styles.cornerRadius` because it duplicates `geometry.cornerRadius`. Use `geometry` as the single source because it already owns the per-corner variants.
7. Omit `styles.padding` when all four values are 0.
8. Omit `styles` when the object is empty (GROUP).

This changes the node-output shape. No golden file pins node output (`internal/testdata/tools_schema.json` pins only the tool schema), so it is safe from current tests. Update the README if it documents the shape.

→ Verify: dump `3659:23362` again, compare the size before and after, and expect a reduction of about 25%.

### Phase P2 — Complete Round-trip

9. **Auto-layout** — emit `styles.layout` when `layoutMode !== "NONE"`:
   `{ mode, itemSpacing, primaryAxisAlignItems, counterAxisAlignItems, primaryAxisSizingMode, counterAxisSizingMode, layoutWrap, counterAxisSpacing }`.
   This enables the `design_token_generation_strategy` prompt to work correctly.
10. **Constraints** — emit `constraints` when they differ from `{MIN, MIN}`.
11. **Paint visibility** — decide as described in Section 4.
12. **`set_paint` accepts gradient `opacity`** — `paintVariants` in `internal/tools_write_modify.go` currently allows only `stops` + `geometry` for gradients, so an emitted `opacity` cannot be written back.

## 4. Decision Required from anh guộc

**How should disabled fills/strokes (eye off) be handled?** `serializePaints` currently does not distinguish them. Two options:

- **Filter them out** — output reflects what is visually visible. This is simple, but a node with only a disabled fill will appear to have no fill, losing information needed for round-tripping.
- **Mark them `visible: false`** — preserve complete information. However, SOLID paints are currently serialized as a bare hex string (`"#ff0000"`), leaving no place for the flag. This would require changing them to object form, adding tokens for every solid fill, or accepting an inconsistent shape.

I lean toward filtering them out because `get_node` is a reading tool for understanding the visual design, not a backup tool.

## 5. Additional Work Completed After Review

- `cssString` folds `opacity` into the alpha of each stop, while `stops[]` retains the raw value so it can round-trip through `set_paint`. Previously, a gradient with opacity 0.6 rendered as fully opaque CSS.
- `serializeEffects` passes through parameters for every effect type. Figma continuously adds types (GLASS, NOISE, TEXTURE); listing only shadow and blur parameters would discard the effect definition. This file contains a `Cosun Glass` style with `depth`/`dispersion`/`lightAngle`/`lightIntensity`/`refraction`; previously it was reduced to `{type, radius}`.
- Omit `children` when a container is empty instead of emitting `children: []`.
- `serializeForDetail` in `read-document.ts` used to calculate `serializeStyles` (including `getStyleByIdAsync`) and discard it before calling `serializeNode` again, doubling style-lookup round trips across the page.
- Vitest test discovery depends on the `--dir src` flag: without it, only 4 of 337 tests are found because `vite.config.ts` sets `root: "./src/ui"` for the single-file UI build. Split out a separate `plugin/vitest.config.ts`. CI is unaffected because it calls `make test-ts`.
- `@vitest/coverage-v8` is an optional peer of Vitest and has never been installed, so `make coverage-ts` hangs at an interactive installation prompt.

## 6. Close the Read/Write Loop for Effects

Read against `@figma/plugin-typings@1.130.0` (`plugin-api.d.ts`, the `Effect` union at line 4442). The union includes DropShadow, InnerShadow, Blur (NORMAL | PROGRESSIVE), Noise (MONOTONE | DUOTONE | MULTITONE), Texture, Glass, and Shader.

Three issues were found by comparing the typings with the code:

1. **`opacity` collision.** `NoiseEffectMultitone` has its own `opacity` field at the effect level, separate from the alpha of `color`. Generic pass-through in `serializeEffects` would overwrite the `opacity` used for alpha. Rename this field to `noiseOpacity`.
2. **Vectors are dropped.** `startOffset`, `endOffset`, and `noiseSizeVector` are Vectors—objects—so pass-through currently accepts only numbers/strings/booleans and drops them. Progressive blur loses both ends of its gradient.
3. **Blur lacks `blurType`.** `BlurEffect` is a discriminated union, but the old write path does not set it and merely casts with `as BlurEffect` to satisfy the compiler.

Write path: consolidate into `makeEffect` in `write-helpers.ts`, support the complete union except SHADER (`figma.importShaderById` is required, so it cannot be constructed from params), and report a dedicated error instead of `"Unknown effect type"`. On the Go side, split `nodeEffectTypes` for `set_effects` while retaining the old four `effectTypes` for `create_style`.

**Runtime quirk**: although `NoiseEffectBase` declares `blendMode`, the Figma runtime rejects that key for every `noiseType`: `Unrecognized key(s) in object: 'blendMode'`. The typings are ahead of the runtime. Therefore, send `blendMode` only when the caller explicitly provides it; do not set a default.

Live verification: write and read back each type (GLASS, NOISE MULTITONE, TEXTURE, progressive blur), then perform one real round trip—read `styles.effects` from node A, write it unchanged to node B, and confirm that both read results match field by field.

## 7. Out of Scope (Recorded, Not Implemented)

- SHADER effect: requires importing the shader by ID and cannot be constructed from params.
- `create_style` with `type: EFFECT` still creates only the four older types. It creates a reusable style rather than an effect on a node, so it is outside this read/write gap.
- SVD calculation of `rotation`/`radius` runs in normalized node space (x by width, y by height), so for a non-square node the principal axes are not the actual geometric axes. The round trip remains consistent because CSS `radial-gradient` also uses percentages on each axis, so reconstruction is not wrong—it is only that the `rotation` number does not have an absolute geometric meaning.
- `vitest.config.ts` does not define `__APP_VERSION__` like `vite.config.ts`. `plugin/src/main.ts` uses this variable, but no current test imports it.
