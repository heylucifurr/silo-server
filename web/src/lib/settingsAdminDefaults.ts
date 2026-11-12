import { SETTING_DEFINITIONS, type SettingKey } from "@/lib/settingsContract";

/**
 * Administrator defaults: a definition marked adminDefault can have its
 * default replaced for the whole server by the server setting
 * `defaults.<key>`, which holds the value as JSON text.
 */
const ADMIN_DEFAULT_PREFIX = "defaults.";

/** The server setting holding the administrator's default for key. */
export function adminDefaultServerKey(key: SettingKey): string {
  return ADMIN_DEFAULT_PREFIX + key;
}

/** The setting whose administrator default serverSettingKey holds, if any. */
export function adminDefaultSettingFor(serverSettingKey: string): SettingKey | undefined {
  if (!serverSettingKey.startsWith(ADMIN_DEFAULT_PREFIX)) return undefined;
  const key = serverSettingKey.slice(ADMIN_DEFAULT_PREFIX.length) as SettingKey;
  return SETTING_DEFINITIONS[key]?.adminDefault ? key : undefined;
}

/**
 * Administrator locks: a definition marked adminLock can be forced to one value
 * for every viewer by the server setting `locks.<key>`, which holds the value
 * as JSON text. Empty means viewers choose for themselves.
 */
const ADMIN_LOCK_PREFIX = "locks.";

/** The policy_input an effective answer names when an administrator lock applies. */
export const ADMIN_LOCK_POLICY_INPUT = "admin_lock";

/** The server setting holding the value the administrator forces for key. */
export function adminLockServerKey(key: SettingKey): string {
  return ADMIN_LOCK_PREFIX + key;
}

/**
 * The setting whose administrator default or lock serverSettingKey holds, if
 * any. Either one changes what viewers resolve.
 */
export function adminOverrideSettingFor(serverSettingKey: string): SettingKey | undefined {
  const fromDefault = adminDefaultSettingFor(serverSettingKey);
  if (fromDefault) return fromDefault;
  if (!serverSettingKey.startsWith(ADMIN_LOCK_PREFIX)) return undefined;
  const key = serverSettingKey.slice(ADMIN_LOCK_PREFIX.length) as SettingKey;
  return SETTING_DEFINITIONS[key]?.adminLock ? key : undefined;
}

/** Whether an effective answer is forced by the server administrator. */
export function isAdminLocked(
  effective: { constrained_by?: { policy_input?: string; constraint?: string } } | undefined,
): boolean {
  return (
    effective?.constrained_by?.policy_input === ADMIN_LOCK_POLICY_INPUT &&
    effective.constrained_by.constraint === "locked"
  );
}

/**
 * The stored administrator default, or the contract default when none is set
 * or the stored text is not JSON, which is also how the server resolves it.
 */
export function storedAdminDefault(key: SettingKey, raw: string | undefined): unknown {
  const fallback = SETTING_DEFINITIONS[key].defaultValue;
  if (!raw) return fallback;
  try {
    return JSON.parse(raw);
  } catch {
    return fallback;
  }
}
