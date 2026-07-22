import React, { useCallback, useEffect, useState } from 'react';
import { Header } from './components/Header';
import { TaskCard } from './components/TaskCard';
import { TaskCreate } from './components/TaskCreate';
import { AuthModal } from './components/AuthModal';
import { ShareModal } from './components/ShareModal';
import type { Task, TaskStatus } from './types';
import { api } from './api/client';
import { useToast } from './context/ToastContext';
import { CheckCircle2, Flame, Layers, Lock, Sparkles } from 'lucide-react';

export const App: React.FC = () => {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [filter, setFilter] = useState<TaskStatus | 'ALL'>('ALL');
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(
      !!localStorage.getItem('access_token')
  );

  const [isAuthOpen, setIsAuthOpen] = useState(false);
  const [isShareOpen, setIsShareOpen] = useState(false);

  const { showToast } = useToast();

  const fetchTasks = useCallback(async () => {
    if (!isAuthenticated) return;
    try {
      const { data } = await api.get('/api/tasks');

      const taskList = Array.isArray(data)
          ? data
          : Array.isArray(data?.tasks)
              ? data.tasks
              : [];

      setTasks(taskList);
    } catch (err: any) {
      showToast('Failed to load tasks', 'error');
      setTasks([]);
    }
  }, [isAuthenticated, showToast]);

  useEffect(() => {
    void fetchTasks();
  }, [fetchTasks]);

  const handleCreateTask = async (title: string, description: string) => {
    if (!isAuthenticated) {
      setIsAuthOpen(true);
      showToast('Please sign in to create tasks', 'info');
      return;
    }
    try {
      const { data } = await api.post('/api/tasks', { title, description });
      setTasks((prev) => [data, ...(Array.isArray(prev) ? prev : [])]);
      showToast('Task added successfully!', 'success');
    } catch (err: any) {
      showToast(err.response?.data?.error || 'Failed to create task', 'error');
    }
  };

  const handleStatusChange = async (id: string, status: TaskStatus) => {
    setTasks((prev) =>
        (Array.isArray(prev) ? prev : []).map((t) =>
            t.id === id ? { ...t, status } : t
        )
    );

    try {
      await api.patch(`/api/tasks/${id}/status`, { status });
    } catch (err: any) {
      showToast('Failed to update task status', 'error');
      await fetchTasks();
    }
  };

  const handleLogout = () => {
    localStorage.clear();
    setIsAuthenticated(false);
    setTasks([]);
    showToast('Logged out successfully', 'info');
  };

  const safeTasks = Array.isArray(tasks) ? tasks : [];

  const filteredTasks = safeTasks.filter((t) =>
      filter === 'ALL' ? true : t.status === filter
  );

  const todoCount = safeTasks.filter((t) => t.status === 'TODO').length;
  const inProgressCount = safeTasks.filter((t) => t.status === 'IN_PROGRESS').length;
  const doneCount = safeTasks.filter((t) => t.status === 'DONE').length;

  return (
      <div className="min-h-screen bg-zinc-950 text-zinc-100 antialiased selection:bg-emerald-500 selection:text-black">
        <Header
            isAuthenticated={isAuthenticated}
            onOpenShare={() => setIsShareOpen(true)}
            onOpenAuth={() => setIsAuthOpen(true)}
            onLogout={handleLogout}
        />

        <main className="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:py-10">
          {isAuthenticated && (
              <div className="mb-8 grid grid-cols-3 gap-2 sm:gap-4">
                <div className="rounded-2xl border border-zinc-800/80 bg-zinc-900/40 p-3 sm:p-4 backdrop-blur-sm">
                  <div className="flex items-center gap-2 text-zinc-400 text-xs font-mono uppercase tracking-wider">
                    <Layers className="h-3.5 w-3.5 text-sky-400" />
                    <span className="hidden sm:inline">Total</span>
                  </div>
                  <p className="mt-1 text-xl sm:text-2xl font-mono font-bold text-zinc-100">{safeTasks.length}</p>
                </div>

                <div className="rounded-2xl border border-zinc-800/80 bg-zinc-900/40 p-3 sm:p-4 backdrop-blur-sm">
                  <div className="flex items-center gap-2 text-zinc-400 text-xs font-mono uppercase tracking-wider">
                    <Flame className="h-3.5 w-3.5 text-amber-400" />
                    <span className="hidden sm:inline">In Progress</span>
                  </div>
                  <p className="mt-1 text-xl sm:text-2xl font-mono font-bold text-amber-400">{inProgressCount + todoCount}</p>
                </div>

                <div className="rounded-2xl border border-zinc-800/80 bg-zinc-900/40 p-3 sm:p-4 backdrop-blur-sm">
                  <div className="flex items-center gap-2 text-zinc-400 text-xs font-mono uppercase tracking-wider">
                    <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" />
                    <span className="hidden sm:inline">Completed</span>
                  </div>
                  <p className="mt-1 text-xl sm:text-2xl font-mono font-bold text-emerald-400">{doneCount}</p>
                </div>
              </div>
          )}

          <div className="mb-8">
            <TaskCreate onCreate={handleCreateTask} />
          </div>

          <div className="mb-6 flex items-center justify-between border-b border-zinc-800/80 pb-3">
            <div className="flex items-center gap-1 overflow-x-auto no-scrollbar py-1">
              {(
                  [
                    { id: 'ALL', label: 'All', count: safeTasks.length },
                    { id: 'TODO', label: 'To Do', count: todoCount },
                    { id: 'IN_PROGRESS', label: 'In Progress', count: inProgressCount },
                    { id: 'DONE', label: 'Done', count: doneCount },
                  ] as const
              ).map((tab) => (
                  <button
                      key={tab.id}
                      onClick={() => setFilter(tab.id as any)}
                      className={`whitespace-nowrap rounded-xl px-3.5 py-1.5 text-xs font-medium transition-all ${
                          filter === tab.id
                              ? 'bg-zinc-100 text-zinc-950 shadow-lg shadow-zinc-100/10'
                              : 'text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200'
                      }`}
                  >
                    {tab.label}
                    <span className={`ml-1.5 text-[10px] font-mono ${filter === tab.id ? 'text-zinc-600' : 'text-zinc-500'}`}>
                  {tab.count}
                </span>
                  </button>
              ))}
            </div>
          </div>

          {!isAuthenticated ? (
              <div className="mt-12 flex flex-col items-center justify-center rounded-3xl border border-zinc-800/80 bg-zinc-900/30 p-8 sm:p-16 text-center backdrop-blur-md">
                <div className="rounded-2xl border border-zinc-800 bg-zinc-900 p-4 text-emerald-400 mb-4 shadow-xl">
                  <Lock className="h-6 w-6" />
                </div>
                <h3 className="text-xl font-bold text-zinc-100 tracking-tight">Workspace Locked</h3>
                <p className="mt-2 text-xs sm:text-sm text-zinc-400 max-w-md leading-relaxed">
                  Sign in to access your personal task manager.
                </p>
                <button
                    onClick={() => setIsAuthOpen(true)}
                    className="mt-6 rounded-xl bg-emerald-500 px-6 py-2.5 text-xs font-bold text-zinc-950 shadow-lg shadow-emerald-500/20 hover:bg-emerald-400 transition active:scale-95"
                >
                  Sign In
                </button>
              </div>
          ) : filteredTasks.length === 0 ? (
              <div className="mt-12 flex flex-col items-center justify-center rounded-3xl border border-dashed border-zinc-800/80 p-12 text-center">
                <Sparkles className="h-8 w-8 text-zinc-600 mb-3" />
                <p className="text-sm font-medium text-zinc-400">No tasks found</p>
                <p className="text-xs text-zinc-600 mt-1">Add a new task above to get started</p>
              </div>
          ) : (
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {filteredTasks.map((task) => (
                    <TaskCard key={task.id} task={task} onStatusChange={handleStatusChange} />
                ))}
              </div>
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