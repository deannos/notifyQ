export interface User {
  id: string;
  username: string;
  is_admin: boolean;
  created_at: string;
}

export interface App {
  id: string;
  user_id: string;
  name: string;
  description: string;
  token?: string;
  created_at: string;
}

export interface Notification {
  id: string;
  app_id: string;
  app?: App;
  title: string;
  message: string;
  priority: number;
  read: boolean;
  created_at: string;
}
