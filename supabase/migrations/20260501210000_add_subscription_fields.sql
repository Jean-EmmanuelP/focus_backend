-- Add subscription tracking fields to users table
-- Synced from iOS StoreKit 2 transactions
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS is_pro boolean DEFAULT false;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS subscription_plan text;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS subscription_expires_at timestamptz;
