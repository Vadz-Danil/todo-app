import React from 'react';
import {
    Command,
    LayoutDashboard,
    LogIn,
    LogOut,
    Moon,
    Share2,
    Sparkles,
    Sun,
    SquareKanban,
    Target,
    User as UserIcon,
} from 'lucide-react';
import { useTheme } from '../context/ThemeContext';

export type AppView = 'board' | 'dashboard' | 'planner' | 'sprints';

const NAV: { id: AppView; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
    { id: 'board', label: 'Board', icon: SquareKanban },
    { id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { id: 'planner', label: 'AI Planner', icon: Sparkles },
    { id: 'sprints', label: 'Sprints', icon: Target },
];

interface HeaderProps {
    isAuthenticated: boolean;
    userEmail?: string;
    view: AppView;
    onViewChange: (view: AppView) => void;
    onOpenShare: () => void;
    onOpenAuth: () => void;
    onLogout: () => void;
}

export const Header: React.FC<HeaderProps> = ({
    isAuthenticated,
    userEmail,
    view,
    onViewChange,
    onOpenShare,
    onOpenAuth,
    onLogout,
}) => {
    const { theme, toggleTheme } = useTheme();

    return (
        <header className="sticky top-0 z-40 w-full border-b border-border bg-bg/70 backdrop-blur-xl transition-colors">
            <div className="mx-auto flex max-w-[1600px] items-center justify-between gap-3 px-4 py-3 sm:px-6">
                <div className="flex min-w-0 items-center gap-2.5">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border bg-hover text-success shadow-inner">
                        <Command className="h-4 w-4" />
                    </div>
                    <span className="hidden text-sm font-bold uppercase tracking-wider text-text-h font-mono sm:inline">
                        Task//Flow
                    </span>
                </div>

                {isAuthenticated && (
                    <nav className="flex min-w-0 flex-1 items-center justify-center gap-1 overflow-x-auto no-scrollbar">
                        {NAV.map((item) => {
                            const Icon = item.icon;
                            const active = view === item.id;
                            return (
                                <button
                                    key={item.id}
                                    onClick={() => onViewChange(item.id)}
                                    aria-current={active ? 'page' : undefined}
                                    className={`flex shrink-0 items-center gap-1.5 rounded-xl px-3 py-1.5 text-xs font-semibold transition active:scale-95 ${
                                        active
                                            ? 'bg-text-h text-bg shadow-md'
                                            : 'text-text hover:bg-hover hover:text-text-h'
                                    }`}
                                >
                                    <Icon className="h-3.5 w-3.5" />
                                    <span className="hidden md:inline">{item.label}</span>
                                </button>
                            );
                        })}
                    </nav>
                )}

                <div className="flex shrink-0 items-center gap-2 sm:gap-3">
                    <button
                        onClick={toggleTheme}
                        title="Toggle theme"
                        aria-label="Toggle theme"
                        className="rounded-xl border border-border bg-hover p-2 text-text transition hover:text-text-h active:scale-95"
                    >
                        {theme === 'dark' ? (
                            <Sun className="h-3.5 w-3.5 text-warning" />
                        ) : (
                            <Moon className="h-3.5 w-3.5 text-text" />
                        )}
                    </button>

                    {isAuthenticated ? (
                        <>
                            <button
                                onClick={onOpenShare}
                                className="flex items-center gap-2 rounded-xl border border-border bg-hover px-3 py-1.5 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95"
                            >
                                <Share2 className="h-3.5 w-3.5 text-success" />
                                <span className="hidden sm:inline">Share</span>
                            </button>

                            <div className="hidden max-w-[180px] items-center gap-2 truncate rounded-xl border border-border/50 bg-hover/60 px-3 py-1.5 font-mono text-xs text-text lg:flex">
                                <UserIcon className="h-3.5 w-3.5 shrink-0 text-text/70" />
                                <span className="truncate">{userEmail || 'user'}</span>
                            </div>

                            <button
                                onClick={onLogout}
                                title="Sign Out"
                                aria-label="Sign out"
                                className="rounded-xl border border-border bg-hover p-2 text-text transition hover:border-danger/40 hover:bg-danger/10 hover:text-danger active:scale-95"
                            >
                                <LogOut className="h-3.5 w-3.5" />
                            </button>
                        </>
                    ) : (
                        <button
                            onClick={onOpenAuth}
                            className="flex items-center gap-2 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-lg shadow-success/10 transition hover:bg-success/90 active:scale-95"
                        >
                            <LogIn className="h-3.5 w-3.5" />
                            <span>Sign In</span>
                        </button>
                    )}
                </div>
            </div>
        </header>
    );
};
