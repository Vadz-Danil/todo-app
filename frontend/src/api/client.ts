import axios from 'axios';

const API_URL = import.meta.env.VITE_API_BASE_URL || '';

export const api = axios.create({
    baseURL: API_URL,
    headers: {
        'Content-Type': 'application/json',
    },
});

api.interceptors.request.use((config) => {
    const token = localStorage.getItem('access_token');
    if (token && config.headers) {
        config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
});

api.interceptors.response.use(
    (response) => response,
    async (error) => {
        const originalRequest = error.config;
        if (!error.response) {
            window.dispatchEvent(
                new CustomEvent('toast', {
                    detail: { message: 'Немає зв’язку з сервером', type: 'error' },
                })
            );
            return Promise.reject(error);
        }
        if (error.response?.status === 401 && !originalRequest._retry) {
            originalRequest._retry = true;

            const refreshToken = localStorage.getItem('refresh_token');
            if (!refreshToken) {
                localStorage.removeItem('access_token');
                localStorage.removeItem('refresh_token');
                window.location.reload();
                return Promise.reject(error);
            }

            try {
                const { data } = await axios.post(`${API_URL}/auth/refresh`, {
                    refresh_token: refreshToken,
                });

                localStorage.setItem('access_token', data.access_token);
                localStorage.setItem('refresh_token', data.refresh_token);

                originalRequest.headers.Authorization = `Bearer ${data.access_token}`;
                return api(originalRequest);
            } catch (refreshErr) {
                localStorage.removeItem('access_token');
                localStorage.removeItem('refresh_token');
                window.location.reload();
                return Promise.reject(refreshErr);
            }
        }

        return Promise.reject(error);
    }
);