import React, { useCallback, useEffect, useState } from 'react';
import { Lock } from 'lucide-react';
import { Header } from './components/Header';
import type { AppView } from './components/Header';
import { AuthModal } from './components/AuthModal';
import { ShareModal } from './components/ShareModal';
import { KanbanBoard } from './components/board/KanbanBoard';
import { Dashboard } from './components/Dashboard';
import { AIPlanner } from './components/AIPlanner';
import { SprintsView } from './components/SprintsView';
import type { Sprint, Task, TaskInput, TaskStatus } from './types';
import {
    createTask as apiCreateTask,
    deleteTask as apiDeleteTask,
    getMe,
    listSprints,
    listTasks,
    moveTask as apiMoveTask,
    updateTask as apiUpdateTask,
} from './api/endpoints';
import { consumeGoogleRedirect } from './api/googleAuth';
import { useToast } from './context/ToastContext';

export const App: React.FC = () => {
    const [tasks, setTasks] = useState<Task[]>([]);
    const [sprints, setSprints] = useState<Sprint[]>([]);
    const [loading, setLoading] = useState(false);
    const [view, setView] = useState<AppView>('board');
    const [userEmail, setUserEmail] = useState<string | undefined>();
    const [isAuthenticated, setIsAuthenticated] = useState<boolean>(
        !!localStorage.getItem('access_token')
    );

    const [isAuthOpen, setIsAuthOpen] = useState(false);
    const [isShareOpen, setIsShareOpen] = useState(false);

    const { showToast } = useToast();

    // Google sends the browser back here with ?code=...; pick it up before the
    // first render decides the user is signed out.
    const [resolvingGoogle, setResolvingGoogle] = useState(
        () => new URLSearchParams(window.location.search).has('code')
    );

    useEffect(() => {
        let cancelled = false;

        void consumeGoogleRedirect().then((result) => {
            if (cancelled || result.status === 'none') {
                setResolvingGoogle(false);
                return;
            }
            if (result.status === 'success') {
                setIsAuthenticated(true);
                showToast('Successfully authenticated with Google', 'success');
            } else {
                showToast(result.message, 'error');
            }
            setResolvingGoogle(false);
        });

        return () => {
            cancelled = true;
        };
        // Runs once: the callback params are stripped as soon as they are read.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const fetchTasks = useCallback(async () => {
        if (!isAuthenticated) return;
        setLoading(true);
        try {
            setTasks(await listTasks());
        } catch {
            showToast('Failed to load tasks', 'error');
            setTasks([]);
        } finally {
            setLoading(false);
        }
    }, [isAuthenticated, showToast]);

    const fetchSprints = useCallback(async () => {
        if (!isAuthenticated) return;
        try {
            setSprints(await listSprints());
        } catch {
            // Sprints are optional context on the board; a failure here is not
            // worth a toast on top of whatever else already failed.
            setSprints([]);
        }
    }, [isAuthenticated]);

    useEffect(() => {
        if (!isAuthenticated) {
            setUserEmail(undefined);
            return;
        }
        void fetchTasks();
        void fetchSprints();
        void getMe()
            .then((me) => setUserEmail(me.email))
            .catch(() => setUserEmail(undefined));
    }, [isAuthenticated, fetchTasks, fetchSprints]);

    const handleCreate = async (input: TaskInput) => {
        if (!isAuthenticated) {
            setIsAuthOpen(true);
            showToast('Please sign in to create tasks', 'info');
            return;
        }
        const created = await apiCreateTask(input);
        setTasks((prev) => [...prev, created]);
        showToast('Task added', 'success');
    };

    const handleUpdate = async (id: string, patch: Partial<TaskInput>) => {
        const updated = await apiUpdateTask(id, patch);
        setTasks((prev) => prev.map((t) => (t.id === id ? updated : t)));
    };

    const handleDelete = async (id: string) => {
        await apiDeleteTask(id);
        setTasks((prev) => prev.filter((t) => t.id !== id));
        showToast('Task deleted', 'info');
    };

    const handleMove = async (
        id: string,
        status: TaskStatus,
        afterId: string | null,
        beforeId: string | null
    ) => {
        const moved = await apiMoveTask(id, status, afterId, beforeId);
        setTasks((prev) => prev.map((t) => (t.id === id ? moved : t)));
    };

    const handleLogout = () => {
        // Not localStorage.clear(): that also drops the theme preference and
        // any other non-credential state the app keeps.
        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        setIsAuthenticated(false);
        setTasks([]);
        setSprints([]);
        setView('board');
        showToast('Logged out successfully', 'info');
    };

    return (
        <div className="min-h-screen bg-bg text-text-h antialiased transition-colors selection:bg-accent selection:text-bg">
            <Header
                isAuthenticated={isAuthenticated}
                userEmail={userEmail}
                view={view}
                onViewChange={setView}
                onOpenShare={() => setIsShareOpen(true)}
                onOpenAuth={() => setIsAuthOpen(true)}
                onLogout={handleLogout}
            />

            <main className="mx-auto w-full max-w-[1600px] px-4 py-6 text-left sm:px-6 lg:py-8">
                {resolvingGoogle ? (
                    <div className="mt-12 flex flex-col items-center justify-center rounded-3xl border border-border bg-surface/60 p-8 text-center shadow-(--shadow) backdrop-blur-md sm:p-16">
                        <div className="h-8 w-8 animate-spin rounded-full border-2 border-border border-t-success" />
                        <p className="mt-4 text-xs text-text sm:text-sm">Finishing sign-in…</p>
                    </div>
                ) : !isAuthenticated ? (
                    <div className="mt-12 flex flex-col items-center justify-center rounded-3xl border border-border bg-surface/60 p-8 text-center shadow-(--shadow) backdrop-blur-md sm:p-16">
                        <div className="mb-4 rounded-2xl border border-border bg-code-bg p-4 text-success shadow-xl">
                            <Lock className="h-6 w-6" />
                        </div>
                        <h3 className="text-xl font-bold tracking-tight text-text-h">Workspace Locked</h3>
                        <p className="mt-2 max-w-md text-xs leading-relaxed text-text sm:text-sm">
                            Sign in to reach your board, analytics and the AI sprint planner.
                        </p>
                        <button
                            onClick={() => setIsAuthOpen(true)}
                            className="mt-6 rounded-xl bg-success px-6 py-2.5 text-xs font-bold text-bg shadow-lg shadow-success/20 transition hover:bg-success/90 active:scale-95"
                        >
                            Sign In
                        </button>
                    </div>
                ) : view === 'board' ? (
                    <KanbanBoard
                        tasks={tasks}
                        sprints={sprints}
                        loading={loading}
                        onCreate={handleCreate}
                        onUpdate={handleUpdate}
                        onDelete={handleDelete}
                        onMove={handleMove}
                    />
                ) : view === 'dashboard' ? (
                    <Dashboard isAuthenticated={isAuthenticated} />
                ) : view === 'planner' ? (
                    <AIPlanner
                        onCommitted={() => {
                            void fetchTasks();
                            void fetchSprints();
                            setView('sprints');
                        }}
                    />
                ) : (
                    <SprintsView onOpenTask={() => setView('board')} />
                )}
            </main>

            <AuthModal
                isOpen={isAuthOpen}
                onClose={() => setIsAuthOpen(false)}
                onSuccess={() => setIsAuthenticated(true)}
            />

            <ShareModal isOpen={isShareOpen} onClose={() => setIsShareOpen(false)} />
        </div>
    );
};

export default App;
