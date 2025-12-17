-- ⚠️ RESET: Drop everything to start fresh
drop table if exists public.focus_sessions cascade;
drop table if exists public.daily_reflections cascade;
drop table if exists public.routine_completions cascade;
drop table if exists public.routines cascade;
drop table if exists public.quests cascade;
drop table if exists public.areas cascade;

-- ==========================================
-- 1. AREAS (User Specific)
-- Each user defines their own buckets
-- ==========================================
create table public.areas (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  
  name text not null,          -- "Health", "Career"
  slug text,                   -- "health"
  -- color column removed
  icon text,                   -- "heart"
  
  completeness integer default 0, -- Stored % value (0-100)
  
  created_at timestamp with time zone default now()
);

-- RLS: Only own data
alter table public.areas enable row level security;
create policy "Users can manage own areas" on public.areas
  using (auth.uid() = user_id);


-- ==========================================
-- 2. QUESTS (Smart Tracking)
-- Tracks 4/12 books, etc.
-- ==========================================
create table public.quests (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  area_id uuid not null references public.areas on delete cascade,
  
  title text not null,         -- "Read 12 Books"
  status text default 'active', -- "active", "completed", "archived"
  
  current_value integer default 0, -- 4
  target_value integer default 1,  -- 12
  
  created_at timestamp with time zone default now()
);

-- RLS
alter table public.quests enable row level security;
create policy "Users can manage own quests" on public.quests
  using (auth.uid() = user_id);


-- ==========================================
-- 3. ROUTINES (Habits)
-- ==========================================
create table public.routines (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  area_id uuid not null references public.areas on delete cascade,
  
  title text not null,         -- "Drink 2L Water"
  frequency text default 'daily', -- "daily", "weekly"
  icon text,                   -- "water-drop"
  
  created_at timestamp with time zone default now()
);

-- RLS
alter table public.routines enable row level security;
create policy "Users can manage own routines" on public.routines
  using (auth.uid() = user_id);


-- ==========================================
-- 4. ROUTINE COMPLETIONS (History Log)
-- Tracks every time a routine is done
-- ==========================================
create table public.routine_completions (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  routine_id uuid not null references public.routines on delete cascade,
  
  completed_at timestamp with time zone default now(),
  
  -- Constraint: Prevents double-logging the same routine on the same day
  unique (user_id, routine_id, completed_at) 
);

-- RLS
alter table public.routine_completions enable row level security;
create policy "Users can manage own completions" on public.routine_completions
  using (auth.uid() = user_id);

-- ==========================================
-- 5. DAILY REFLECTIONS (Journaling)
-- ==========================================
create table public.daily_reflections (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  
  -- The specific date this entry is for (e.g., '2023-10-27')
  date date default current_date not null,
  
  -- The 4 Input Fields
  biggest_win text,           -- "What made you proud today?"
  challenges text,            -- "What blocked or stressed you?"
  best_moment text,           -- "A moment that made you smile..."
  goal_for_tomorrow text,     -- "What will you focus on tomorrow?"
  
  created_at timestamp with time zone default now(),
  
  -- Constraint: Only one reflection per user per day.
  unique (user_id, date)
);

-- RLS: Only own data
alter table public.daily_reflections enable row level security;

create policy "Users can manage own reflections" 
on public.daily_reflections
using (auth.uid() = user_id);

-- ==========================================
-- 6. FOCUS SESSIONS (Pomodoro / Deep Work)
-- ==========================================
create table public.focus_sessions (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  
  -- Optional link to a specific Quest
  -- "on delete set null" means if the Quest is deleted, we keep the session history, just unlink it.
  quest_id uuid references public.quests on delete set null,
  
  description text,              -- "What will you work on?"
  duration_minutes integer not null, -- 25, 50, 90, etc.
  
  -- Status tracking
  status text default 'active',  -- 'active', 'completed', 'cancelled'
  
  -- Timestamps
  started_at timestamp with time zone default now(),
  completed_at timestamp with time zone, -- Null until finished
  
  created_at timestamp with time zone default now()
);

-- RLS
alter table public.focus_sessions enable row level security;

create policy "Users can manage own sessions"
on public.focus_sessions
using (auth.uid() = user_id);


-- ==========================================
-- 7. TASKS (Calendar Tasks - unified with time_blocks)
-- ==========================================
create table public.tasks (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  -- Optional links
  quest_id uuid references public.quests on delete set null,
  area_id uuid references public.areas on delete set null,

  -- Core fields
  title text not null,
  description text,
  position integer default 0,

  -- Time estimates
  estimated_minutes integer,
  actual_minutes integer default 0,

  -- Priority and status
  priority text default 'medium',  -- low, medium, high, urgent
  status text default 'pending',   -- pending, in_progress, completed

  -- Calendar scheduling
  date date default current_date,
  scheduled_start time,            -- HH:mm format in DB
  scheduled_end time,              -- HH:mm format in DB
  time_block text default 'morning', -- morning, afternoon, evening

  -- Due date and completion
  due_at timestamp with time zone,
  completed_at timestamp with time zone,

  -- AI fields
  is_ai_generated boolean default false,
  ai_notes text,

  -- Timestamps
  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now()
);

-- RLS
alter table public.tasks enable row level security;
create policy "Users can manage own tasks" on public.tasks
  using (auth.uid() = user_id);

-- Indexes for performance
create index idx_tasks_user_date on public.tasks(user_id, date);
create index idx_tasks_quest on public.tasks(quest_id);
create index idx_tasks_area on public.tasks(area_id);


-- ==========================================
-- 8. DAY PLANS (For AI day planning)
-- ==========================================
create table public.day_plans (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  date date not null,
  ideal_day_prompt text,
  ai_summary text,
  progress integer default 0,
  status text default 'active',

  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now(),

  unique (user_id, date)
);

-- RLS
alter table public.day_plans enable row level security;
create policy "Users can manage own day_plans" on public.day_plans
  using (auth.uid() = user_id);


-- ==========================================
-- 9. FRIEND GROUPS (Custom friend groups)
-- ==========================================
create table public.friend_groups (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  name text not null,            -- "Gym Buddies", "Work Team"
  description text,              -- Optional description
  icon text default '👥',        -- Emoji icon
  color text default '#6366F1',  -- Hex color for display

  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now()
);

-- RLS: Only own groups
alter table public.friend_groups enable row level security;
create policy "Users can manage own friend_groups" on public.friend_groups
  using (auth.uid() = user_id);

-- Indexes
create index idx_friend_groups_user on public.friend_groups(user_id);


-- ==========================================
-- 10. FRIEND GROUP MEMBERS (Members in groups)
-- ==========================================
create table public.friend_group_members (
  id uuid default gen_random_uuid() primary key,
  group_id uuid not null references public.friend_groups on delete cascade,
  member_id uuid not null references auth.users on delete cascade,

  added_at timestamp with time zone default now(),

  -- Prevent duplicate members in same group
  unique (group_id, member_id)
);

-- RLS: Users can manage members of their own groups
alter table public.friend_group_members enable row level security;
create policy "Users can manage own friend_group_members" on public.friend_group_members
  using (
    exists (
      select 1 from public.friend_groups g
      where g.id = group_id and g.user_id = auth.uid()
    )
  );

-- Indexes
create index idx_friend_group_members_group on public.friend_group_members(group_id);
create index idx_friend_group_members_member on public.friend_group_members(member_id);


-- ==========================================
-- 11. COMMUNITY POSTS (Social Feed)
-- ==========================================
create table public.community_posts (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  -- Link to task OR routine (at least one required)
  task_id uuid references public.tasks on delete set null,
  routine_id uuid references public.routines on delete set null,

  -- Content
  image_url text not null,           -- URL to Supabase Storage
  caption text,                      -- Optional description

  -- Engagement
  likes_count integer default 0,

  -- Moderation
  is_hidden boolean default false,   -- Hidden if reported & reviewed

  created_at timestamp with time zone default now()
  -- task_id and routine_id are optional
);

-- RLS: Public read (non-hidden), own write
alter table public.community_posts enable row level security;

-- Anyone can read visible posts
create policy "Anyone can read visible posts" on public.community_posts
  for select using (is_hidden = false);

-- Users can manage own posts
create policy "Users can manage own posts" on public.community_posts
  for all using (auth.uid() = user_id);

-- Indexes
create index idx_community_posts_user on public.community_posts(user_id);
create index idx_community_posts_created on public.community_posts(created_at desc);
create index idx_community_posts_task on public.community_posts(task_id) where task_id is not null;
create index idx_community_posts_routine on public.community_posts(routine_id) where routine_id is not null;
create index idx_community_posts_visible on public.community_posts(created_at desc) where is_hidden = false;


-- ==========================================
-- 12. COMMUNITY POST LIKES
-- ==========================================
create table public.community_post_likes (
  id uuid default gen_random_uuid() primary key,
  post_id uuid not null references public.community_posts on delete cascade,
  user_id uuid not null references auth.users on delete cascade,

  created_at timestamp with time zone default now(),

  -- Prevent duplicate likes
  unique (post_id, user_id)
);

-- RLS
alter table public.community_post_likes enable row level security;

create policy "Anyone can read likes" on public.community_post_likes
  for select using (true);

create policy "Users can manage own likes" on public.community_post_likes
  for all using (auth.uid() = user_id);

-- Indexes
create index idx_community_post_likes_post on public.community_post_likes(post_id);
create index idx_community_post_likes_user on public.community_post_likes(user_id);


-- ==========================================
-- 13. COMMUNITY POST REPORTS (Moderation)
-- ==========================================
create table public.community_post_reports (
  id uuid default gen_random_uuid() primary key,
  post_id uuid not null references public.community_posts on delete cascade,
  reporter_id uuid not null references auth.users on delete cascade,

  reason text not null,              -- 'inappropriate', 'spam', 'harassment', 'other'
  details text,                      -- Optional additional details
  status text default 'pending',     -- 'pending', 'reviewed', 'dismissed'

  created_at timestamp with time zone default now(),

  -- One report per user per post
  unique (post_id, reporter_id)
);

-- RLS: Only admins should review reports, but users can create their own
alter table public.community_post_reports enable row level security;

create policy "Users can create own reports" on public.community_post_reports
  for insert with check (auth.uid() = reporter_id);

create policy "Users can view own reports" on public.community_post_reports
  for select using (auth.uid() = reporter_id);

-- Index for pending reports
create index idx_community_post_reports_pending on public.community_post_reports(created_at) where status = 'pending';