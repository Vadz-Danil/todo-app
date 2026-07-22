import React, { createContext, useContext, useState, useCallback } from 'react';
import { AlertCircle, CheckCircle2, Info, X } from 'lucide-react';

export type ToastType = 'success' | 'error' | 'info';

interface Toast {
    id: string;
    message: string;
    type: ToastType;
}

interface ToastContextType {
    showToast: (message: string, type?: ToastType) => void;
}

const ToastContext = createContext<ToastContextType | undefined>(undefined);

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [toasts, setToasts] = useState<Toast[]>([]);

    const removeToast = useCallback((id: string) => {
        setToasts((prev) => prev.filter((toast) => toast.id !== id));
    }, []);

    const showToast = useCallback((message: string, type: ToastType = 'info') => {
        const id = Math.random().toString(36).substring(2, 9);

        setToasts((prev) => [...prev, { id, message, type }]);

        setTimeout(() => {
            removeToast(id);
        }, 3500);
    }, [removeToast]);

    return (
        <ToastContext.Provider value={{ showToast }}>
            {children}

            {/* Контейнер Pop-Down сповіщень (зверху по центру екрана) */}
            <div className="fixed top-5 left-1/2 -translate-x-1/2 z-100 flex flex-col gap-2 w-full max-w-sm px-4 pointer-events-none">
                {toasts.map((toast) => (
                    <div
                        key={toast.id}
                        className={`pointer-events-auto flex items-center justify-between gap-3 rounded-2xl border p-3.5 shadow-2xl backdrop-blur-xl transition-all duration-300 animate-in fade-in slide-in-from-top-5 ${
                            toast.type === 'error'
                                ? 'bg-rose-950/90 border-rose-800/80 text-rose-200'
                                : toast.type === 'success'
                                    ? 'bg-emerald-950/90 border-emerald-800/80 text-emerald-200'
                                    : 'bg-zinc-900/90 border-zinc-800 text-zinc-200'
                        }`}
                    >
                        <div className="flex items-center gap-2.5 min-w-0">
                            {toast.type === 'error' && <AlertCircle className="h-4 w-4 shrink-0 text-rose-400" />}
                            {toast.type === 'success' && <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-400" />}
                            {toast.type === 'info' && <Info className="h-4 w-4 shrink-0 text-sky-400" />}

                            <span className="text-xs font-medium leading-tight truncate">
                                {toast.message}
                            </span>
                        </div>

                        <button
                            onClick={() => removeToast(toast.id)}
                            className="rounded-lg p-1 opacity-70 hover:opacity-100 transition"
                        >
                            <X className="h-3.5 w-3.5" />
                        </button>
                    </div>
                ))}
            </div>
        </ToastContext.Provider>
    );
};

export const useToast = () => {
    const context = useContext(ToastContext);
    if (!context) {
        throw new Error('useToast must be used within a ToastProvider');
    }
    return context;
};