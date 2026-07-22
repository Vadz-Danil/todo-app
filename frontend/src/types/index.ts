export type TaskStatus = 'TODO' | 'IN_PROGRESS' | 'DONE';

export interface Task {
    id: string;
    user_id?: string;
    title: string;
    description?: string;
    status: TaskStatus;
    created_at?: string;
}
