import { api } from './client';

// Google's authorization-code flow, driven as a full-page redirect rather than
// a popup.
//
// The popup variant (`ux_mode: 'popup'` / `redirect_uri=postmessage`) cannot
// work on phones: mobile browsers ignore window features and open a plain tab,
// iOS Safari's tracking prevention partitions storage for it, and the
// opener relationship the postMessage handshake depends on is not preserved.
// A redirect has none of those problems and also survives popup blockers on
// desktop, so it is used everywhere.

const AUTH_ENDPOINT = 'https://accounts.google.com/o/oauth2/v2/auth';
const STATE_KEY = 'google_oauth_state';

// Must match the scopes the backend requests when it exchanges the code.
const SCOPES = [
    'https://www.googleapis.com/auth/userinfo.email',
    'https://www.googleapis.com/auth/userinfo.profile',
].join(' ');

/**
 * The redirect target handed to Google. It must match an Authorized redirect
 * URI in Google Cloud Console byte for byte, so it is the bare origin with no
 * trailing slash.
 */
export function googleRedirectURI(): string {
    return window.location.origin.replace(/\/$/, '');
}

function randomState(): string {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export function isGoogleConfigured(): boolean {
    return !!import.meta.env.VITE_GOOGLE_CLIENT_ID;
}

/** Sends the browser to Google's consent screen. Does not return. */
export function startGoogleLogin(): void {
    const clientId = import.meta.env.VITE_GOOGLE_CLIENT_ID;
    if (!clientId) {
        throw new Error('VITE_GOOGLE_CLIENT_ID is not configured');
    }

    // state is the CSRF defence: only a response carrying the value we just
    // generated is one we actually asked for.
    const state = randomState();
    sessionStorage.setItem(STATE_KEY, state);

    const params = new URLSearchParams({
        client_id: clientId,
        redirect_uri: googleRedirectURI(),
        response_type: 'code',
        scope: SCOPES,
        state,
        include_granted_scopes: 'true',
        prompt: 'select_account',
    });

    window.location.assign(`${AUTH_ENDPOINT}?${params.toString()}`);
}

export type GoogleCallbackResult =
    | { status: 'none' }
    | { status: 'success' }
    | { status: 'error'; message: string };

/**
 * Consumes a `?code=...&state=...` (or `?error=...`) callback if one is present
 * on the current URL, exchanging the code for this app's tokens.
 *
 * The query string is stripped either way, so a reload cannot replay a code
 * that Google has already invalidated.
 */
export async function consumeGoogleRedirect(): Promise<GoogleCallbackResult> {
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code');
    const state = params.get('state');
    const error = params.get('error');

    if (!code && !error) {
        return { status: 'none' };
    }

    const expectedState = sessionStorage.getItem(STATE_KEY);
    sessionStorage.removeItem(STATE_KEY);
    clearCallbackParams();

    if (error) {
        return {
            status: 'error',
            message: error === 'access_denied' ? 'Sign-in was cancelled' : `Google returned: ${error}`,
        };
    }

    if (!expectedState || state !== expectedState) {
        return { status: 'error', message: 'Sign-in could not be verified. Please try again.' };
    }

    try {
        const { data } = await api.post('/auth/google', {
            code,
            // The token endpoint compares this against the redirect_uri used in
            // the authorization request and rejects any mismatch.
            redirect_uri: googleRedirectURI(),
        });

        localStorage.setItem('access_token', data.access_token);
        localStorage.setItem('refresh_token', data.refresh_token);
        return { status: 'success' };
    } catch (err) {
        const message =
            (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
            'Google authentication failed';
        return { status: 'error', message };
    }
}

/** Removes the OAuth params while leaving any others, and keeps history clean. */
function clearCallbackParams(): void {
    const url = new URL(window.location.href);
    ['code', 'state', 'error', 'scope', 'authuser', 'prompt', 'hd'].forEach((key) =>
        url.searchParams.delete(key)
    );
    window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`);
}
