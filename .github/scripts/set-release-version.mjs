import { readFile, writeFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

export function productVersion(value) {
  const normalized = String(value || "")
    .trim()
    .replace(/^v/, "");
  const match = normalized.match(
    /^(\d+)\.(\d+)\.(\d+)(?:[.+-][0-9A-Za-z.-]+)?$/,
  );
  if (!match) {
    throw new Error(`Invalid release version: ${value}`);
  }
  return `${match[1]}.${match[2]}.${match[3]}`;
}

export async function updateWailsVersion(value, path = "wails.json") {
  const config = JSON.parse(await readFile(path, "utf8"));
  config.info = {
    ...(config.info || {}),
    productVersion: productVersion(value),
  };
  await writeFile(path, `${JSON.stringify(config, null, 2)}\n`, "utf8");
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  await updateWailsVersion(process.argv[2]);
}
