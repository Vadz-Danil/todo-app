import React from 'react';
import { LogOut, Share2, User as UserIcon, LogIn, Command } from 'lucide-react';

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
    return (
        <header className="sticky top-0 z-40 w-full border-b border-zinc-800/80 bg-zinc-950/70 backdrop-blur-xl">
            <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3 sm:px-6">
                <div className="flex items-center gap-2.5">
                    <div className="flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-700 bg-zinc-900 text-emerald-400 shadow-inner">
                        <Command className="h-4 w-4" />
                    </div>
                    <span className="text-sm font-bold tracking-wider text-zinc-100 uppercase font-mono">
                        Task//Flow
                    </span>
                </div>

                <div className="flex items-center gap-2 sm:gap-3">
                    {isAuthenticated ? (
                        <>
                            <button
                                onClick={onOpenShare}
                                className="flex items-center gap-2 rounded-xl border border-zinc-800 bg-zinc-900/80 px-3 py-1.5 text-xs font-semibold text-zinc-200 transition hover:border-zinc-700 hover:bg-zinc-800 active:scale-95"
                            >
                                <Share2 className="h-3.5 w-3.5 text-emerald-400" />
                                <span className="hidden sm:inline">Share</span>
                            </button>

                            <div className="hidden items-center gap-2 rounded-xl border border-zinc-800/50 bg-zinc-900/40 px-3 py-1.5 text-xs font-mono text-zinc-400 md:flex">
                                <UserIcon className="h-3.5 w-3.5 text-zinc-500" />
                                <span>{userEmail || 'user'}</span>
                            </div>

                            <button
                                onClick={onLogout}
                                title="Sign Out"
                                className="rounded-xl border border-zinc-800 bg-zinc-900/80 p-2 text-zinc-400 hover:border-rose-900/50 hover:bg-rose-950/30 hover:text-rose-400 transition active:scale-95"
                            >
                                <LogOut className="h-3.5 w-3.5" />
                            </button>
                        </>
                    ) : (
                        <button
                            onClick={onOpenAuth}
                            className="flex items-center gap-2 rounded-xl bg-emerald-500 px-4 py-2 text-xs font-bold text-zinc-950 shadow-lg shadow-emerald-500/10 hover:bg-emerald-400 transition active:scale-95"
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