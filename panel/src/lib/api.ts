import { PUBLIC_API_URL } from '$env/static/public';
import { goto } from '$app/navigation';

export function getToken(): string | null {
	return sessionStorage.getItem('token');
}

export function setToken(token: string): void {
	sessionStorage.setItem('token', token);
}

export function clearToken(): void {
	sessionStorage.removeItem('token');
	sessionStorage.removeItem('user_role');
}

export function getUserRole(): string {
	return sessionStorage.getItem('user_role') || '';
}

export function setUserRole(role: string): void {
	sessionStorage.setItem('user_role', role);
}

export function isAuthenticated(): boolean {
	return !!getToken();
}

export async function api(path: string, options: RequestInit = {}): Promise<Response> {
	const token = getToken();
	if (!token) {
		clearToken();
		goto('/login');
		throw new Error('Not authenticated');
	}

	const headers = {
		'Authorization': `Bearer ${token}`,
		...options.headers
	};

	const response = await fetch(`${PUBLIC_API_URL}${path}`, {
		...options,
		headers
	});

	if (response.status === 401) {
		clearToken();
		goto('/login');
		throw new Error('Session expired');
	}

	return response;
}

export async function apiJson<T = unknown>(path: string, options: RequestInit = {}): Promise<T> {
	const response = await api(path, options);
	const data = await response.json();

	if (!response.ok) {
		throw new Error(data.error || data.message || 'Request failed');
	}

	return data as T;
}
