-- ==========================================
-- WHATSAPP INTEGRATION MIGRATIONS
-- Run after main migrations.sql
-- ==========================================

-- ==========================================
-- 1. Add phone_number to users table
-- ==========================================
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS phone_number text UNIQUE;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS phone_verified boolean DEFAULT false;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS whatsapp_linked_at timestamp with time zone;

-- Index for fast phone lookup
CREATE INDEX IF NOT EXISTS idx_users_phone ON public.users(phone_number) WHERE phone_number IS NOT NULL;

-- ==========================================
-- 2. Add source column to chat_messages
-- ==========================================
ALTER TABLE public.chat_messages ADD COLUMN IF NOT EXISTS source text DEFAULT 'app';
-- 'app' = from iOS app
-- 'whatsapp' = from WhatsApp

-- ==========================================
-- 3. Phone linking OTPs table
-- ==========================================
CREATE TABLE IF NOT EXISTS public.phone_linking_otps (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,
  phone_number text NOT NULL,
  otp_code text NOT NULL,
  expires_at timestamp with time zone NOT NULL,
  verified boolean DEFAULT false,
  created_at timestamp with time zone DEFAULT now()
);

-- Index for cleanup of expired OTPs
CREATE INDEX IF NOT EXISTS idx_otp_expires ON public.phone_linking_otps(expires_at);
CREATE INDEX IF NOT EXISTS idx_otp_phone ON public.phone_linking_otps(phone_number);

-- RLS
ALTER TABLE public.phone_linking_otps ENABLE ROW LEVEL SECURITY;
CREATE POLICY "Users can manage own OTPs" ON public.phone_linking_otps
  USING (auth.uid() = user_id);

-- ==========================================
-- 4. WhatsApp pending users table
-- For users who start via WhatsApp before creating a full account
-- ==========================================
CREATE TABLE IF NOT EXISTS public.whatsapp_pending_users (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  phone_number text UNIQUE NOT NULL,
  display_name text,
  onboarding_step text DEFAULT 'welcome',
  created_at timestamp with time zone DEFAULT now(),
  converted_to_user_id uuid REFERENCES auth.users ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_pending_phone ON public.whatsapp_pending_users(phone_number);

-- ==========================================
-- 5. Daily intentions table (if not exists)
-- For mood logging via WhatsApp
-- ==========================================
CREATE TABLE IF NOT EXISTS public.daily_intentions (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,
  date date DEFAULT current_date NOT NULL,
  mood_rating integer,  -- 1-5
  sleep_rating integer, -- 1-10
  intentions text,
  created_at timestamp with time zone DEFAULT now(),
  UNIQUE (user_id, date)
);

ALTER TABLE public.daily_intentions ENABLE ROW LEVEL SECURITY;
CREATE POLICY "Users can manage own intentions" ON public.daily_intentions
  USING (auth.uid() = user_id);

-- ==========================================
-- 6. Cleanup job for expired OTPs
-- Run periodically via cron
-- ==========================================
-- DELETE FROM public.phone_linking_otps WHERE expires_at < NOW();

-- ==========================================
-- 7. Helper function to link WhatsApp
-- ==========================================
CREATE OR REPLACE FUNCTION link_whatsapp_account(
  p_user_id uuid,
  p_phone_number text
) RETURNS boolean AS $$
BEGIN
  -- Update user with phone number
  UPDATE public.users
  SET phone_number = p_phone_number,
      phone_verified = true,
      whatsapp_linked_at = now()
  WHERE id = p_user_id;

  -- Mark pending user as converted
  UPDATE public.whatsapp_pending_users
  SET converted_to_user_id = p_user_id
  WHERE phone_number = p_phone_number;

  RETURN true;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
