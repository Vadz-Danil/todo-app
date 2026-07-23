import React from 'react';
import { LogOut, Share2, User as UserIcon, LogIn, Command, Sun, Moon } from 'lucide-react';
import { useTheme } from '../context/ThemeContext';

interface HeaderProps {
    isAuthenticated: boolean;
    userEmail?: string;
    onOpenShare: () => void;
    onOpenAuth: () => void;
    onLogout: () => void;
}

export const Header: React.FC<HeaderProps> = ({
                                                  isAuthenticated,
                                                  userEmail,
                                                  onOpenShare,
                                                  onOpenAuth,
                                                  onLogout,
                                              }) => {
    const { theme, toggleTheme } = useTheme();

    return (
        <header className="sticky top-0 z-40 w-full border-b border-border bg-bg/70 backdrop-blur-xl transition-colors">
            <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3 sm:px-6">
                <div className="flex items-center gap-2.5">
                    <div className="flex h-9 w-9 items-center justify-center rounded-xl border border-border bg-hover text-success shadow-inner">
                        <Command className="h-4 w-4" />
                    </div>
                    <span className="text-sm font-bold tracking-wider text-text-h uppercase font-mono">
                        Task//Flow
                    </span>
                </div>

                <div className="flex items-center gap-2 sm:gap-3">
                    {}
                    <button
                        onClick={toggleTheme}
                        title="Toggle theme"
                        className="rounded-xl border border-border bg-hover p-2 text-text hover:text-text-h transition active:scale-95"
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
                                className="flex items-center gap-2 rounded-xl border border-border bg-hover px-3 py-1.5 text-xs font-semibold text-text-h hover:border-text-h/20 transition active:scale-95"
                            >
                                <Share2 className="h-3.5 w-3.5 text-success" />
                                <span className="hidden sm:inline">Share</span>
                            </button>

                            <div className="hidden items-center gap-2 rounded-xl border border-border/50 bg-hover/60 px-3 py-1.5 text-xs font-mono text-text md:flex">
                                <UserIcon className="h-3.5 w-3.5 text-text/70" />
                                <span>{userEmail || 'user'}</span>
                            </div>

                            <button
                                onClick={onLogout}
                                title="Sign Out"
                                className="rounded-xl border border-border bg-hover p-2 text-text hover:border-danger/40 hover:bg-danger/10 hover:text-danger transition active:scale-95"
                            >
                                <LogOut className="h-3.5 w-3.5" />
                            </button>
                        </>
                    ) : (
                        <button
                            onClick={onOpenAuth}
                            className="flex items-center gap-2 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-lg shadow-success/10 hover:bg-success/90 transition active:scale-95"
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