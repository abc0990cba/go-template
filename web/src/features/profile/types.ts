export interface AuthIdentity {
  user_id: string;
  provider: "password" | "google" | "github" | "facebook" | "twitter";
  provider_subject: string;
  email: string;
  name: string;
  avatar_url: string;
  linked_at: string;
  last_login_at: string;
}

export interface OAuthProviderInfo {
  name: string;
  configured: boolean;
}

export interface APIKey {
  id: string;
  user_id: string;
  name: string;
  prefix: string;
  scopes: string[];
  last_used_at: string | null;
  expires_at: string | null;
  revoked_at: string | null;
  created_at: string;
}
