import React from 'react';
import ReactDOM from 'react-dom/client';
import { GoogleOAuthProvider } from '@react-oauth/google';
import App from './App';
import { ToastProvider } from './context/ToastContext';
import './index.css';

const googleClientId = import.meta.env.VITE_GOOGLE_CLIENT_ID || '';

ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
        <GoogleOAuthProvider clientId={googleClientId}>
            <ToastProvider>
                <App />
            </ToastProvider>
        </GoogleOAuthProvider>
    </React.StrictMode>
);