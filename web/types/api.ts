// Shared API response types — keep aligned with the Go backend's JSON shapes
// in internal/publications, internal/sources, internal/issuesapi.

export interface Publication {
  id: string;
  account_id: string;
  name: string;
  brief: string;
  timezone: string;
  cadence_rule: string | null;
  stories_per_issue_min: number;
  stories_per_issue_max: number;
  intro_enabled: boolean;
  curation_lead_time: string; // Go duration, e.g. "24h0m0s"
  approval_gate_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface PublicationListResp {
  items: Publication[];
  next_cursor: string | null;
}

export interface PublicationCreateInput {
  name: string;
  brief?: string;
  timezone: string;
  cadence_rule?: string;
  stories_per_issue_min?: number;
  stories_per_issue_max?: number;
  intro_enabled?: boolean;
  curation_lead_time?: string;
  approval_gate_enabled?: boolean;
}

export interface PublicationUpdateInput {
  name?: string;
  brief?: string;
  timezone?: string;
  cadence_rule?: string;
  unset_cadence_rule?: boolean;
  stories_per_issue_min?: number;
  stories_per_issue_max?: number;
  intro_enabled?: boolean;
  curation_lead_time?: string;
  approval_gate_enabled?: boolean;
}

export interface Source {
  id: string;
  publication_id: string;
  type: "rss" | "youtube_channel" | "x_handle" | "substack" | "web";
  identifier: string;
  poll_interval: string;
  enabled: boolean;
  last_polled_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface SourceListResp {
  items: Source[];
}

export interface IssueSummary {
  id: string;
  state:
    | "planned"
    | "curating"
    | "drafted"
    | "approved"
    | "sending"
    | "sent"
    | "failed"
    | "skipped";
  subject: string | null;
  scheduled_at: string;
  sent_at: string | null;
  cover_url: string | null;
  story_count: number;
}

export interface IssueListResp {
  items: IssueSummary[];
  next_cursor: string | null;
}
