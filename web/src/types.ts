export type UserRole = string;
export type UserStatus = "active" | "disabled";

export interface User {
  id: number;
  username: string;
  display_name: string;
  role: UserRole;
  role_name?: string;
  status: UserStatus;
  created_at: string;
  wechat_bound?: boolean;
  has_password?: boolean;
  ranking_hidden?: boolean;
  permissions?: string[];
  managed_space_ids?: string[];
  joined_space_ids?: string[];
}

export interface Role {
  key: string;
  name: string;
  description: string;
  permissions: string[];
  system: boolean;
  user_count: number;
  created_at: string;
  updated_at: string;
}

export interface PermissionDefinition {
  key: string;
  name: string;
  description: string;
  group: string;
}

export interface AdminOverview {
  users: User[];
  counts: Record<string, number>;
  spaces: AdminSpaceSummary[];
  platform_counts: AdminPlatformCounts;
  registration_enabled: boolean;
  permissions: string[];
  roles: Role[];
  permission_catalog: PermissionDefinition[];
}

export interface AdminSpaceSummary {
  id: string;
  name: string;
  owner_username: string;
  owner_display_name: string;
  newapi_base_url: string;
  admin_token_last4: string;
  quota_per_usd: number;
  created_at: string;
  member_count: number;
  bound_member_count: number;
  table_count: number;
  operation_count: number;
  failed_operations: number;
}

export interface AdminPlatformCounts {
  spaces: number;
  memberships: number;
  bound_memberships: number;
  tables: number;
  operations: number;
  failed_operations: number;
}

export interface Space {
  id: string;
  name: string;
  invite_code?: string;
  owner_user_id: number;
  newapi_base_url: string;
  admin_token_last4: string;
  quota_per_usd: number;
  is_owner: boolean;
  can_manage: boolean;
  is_bound: boolean;
}

export interface Membership {
  newapi_user_id?: number;
  newapi_username?: string;
  newapi_display_name?: string;
  user_token_last4?: string;
  poker_display_name: string;
}

export interface AccountBinding {
  space: Space;
  membership: Membership;
}

export interface Card {
  rank: number;
  suit: number;
}

export type PokerAction = "fold" | "check" | "call" | "bet" | "raise" | "all_in";

export interface Player {
  user_id: number;
  name: string;
  seat: number;
  stack_cents: number;
  bet_cents: number;
  cards?: Card[];
  in_hand: boolean;
  folded: boolean;
  all_in: boolean;
  is_dealer: boolean;
  is_acting: boolean;
  last_action?: PokerAction;
  last_action_amount_cents?: number;
  last_action_bet_level?: number;
}

export interface AllowedActions {
  can_act: boolean;
  can_fold: boolean;
  can_check: boolean;
  can_call: boolean;
  can_bet: boolean;
  can_raise: boolean;
  can_all_in: boolean;
  to_call_cents: number;
  min_raise_to_cents: number;
  max_raise_to_cents: number;
}

export interface TableState {
  id: string;
  name: string;
  small_blind_cents: number;
  big_blind_cents: number;
  hand_id: number;
  street: string;
  board: Card[];
  pot_cents: number;
  current_bet_cents: number;
  bet_level: number;
  dealer_seat: number;
  small_blind_seat: number;
  big_blind_seat: number;
  acting_seat: number;
  viewer_seat: number;
  players: Player[];
  allowed_actions: AllowedActions;
  can_start: boolean;
  can_leave: boolean;
  last_result?: {
    hand_id?: number;
    message: string;
    pot_cents: number;
    showdown?: boolean;
    payouts?: Record<string, number>;
  };
}

export interface TableSummary {
  id: string;
  name: string;
  small_blind_cents: number;
  big_blind_cents: number;
  player_count: number;
  max_players: number;
  hand_id: number;
  street: string;
  viewer_seated: boolean;
  players: TableSeatSummary[];
}

export interface TableSeatSummary {
  user_id: number;
  name: string;
  seat: number;
  stack_cents: number;
}

export interface Balance {
  cents: number;
  quota: number;
  quota_per_usd: number;
}

export interface WalletOperation {
  id: string;
  table_id: string;
  kind: "buy_in" | "cash_out" | "manual_credit" | "manual_debit";
  cents: number;
  actor_user_id?: number;
  note?: string;
  status: string;
  error?: string;
  created_at: string;
}

export interface ChannelLeaderboardEntry {
  user_id: number;
  display_name: string;
  net_cents: number;
  sessions: number;
}

export interface ManagedBalanceMember {
  user_id: number;
  poker_display_name: string;
  newapi_user_id?: number;
  newapi_username?: string;
  newapi_display_name?: string;
  bound: boolean;
  balance?: Balance;
  error?: string;
}

export interface ManagedBalancesResponse {
  space: { id: string; name: string; quota_per_usd: number };
  members: ManagedBalanceMember[];
}
