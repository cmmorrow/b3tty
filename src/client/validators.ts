export const MAX_UINT16 = 65535;

/**
 * Returns true if protocol is a valid HTTP protocol string ("http" or "https").
 */
export function isValidHttpProtocol(protocol: string): boolean {
    return protocol === "http" || protocol === "https";
}

/**
 * Returns true if protocol is a valid WebSocket protocol string ("ws" or "wss").
 */
export function isValidWsProtocol(protocol: string): boolean {
    return protocol === "ws" || protocol === "wss";
}

/**
 * Returns true if port is an integer in the valid TCP port range [1, 65535].
 */
export function isValidPort(port: number): boolean {
    return Number.isInteger(port) && port >= 1 && port <= MAX_UINT16;
}

/**
 * Returns true if uri is a valid hostname or IPv4 address.
 * Each dot-separated label must start and end with an alphanumeric character
 * and may contain hyphens. Bare single-label names (e.g. "localhost") are
 * also accepted.
 */
export function isValidUri(uri: string): boolean {
    if (!uri) return false;
    const labelRe = /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$/;
    return uri.split(".").every((label) => labelRe.test(label));
}
