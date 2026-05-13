export interface Skill {
  id: number;
  user_id: number;
  skill_key: string;
  name: string;
  description?: string;
  status: string;
  source_type: string;
  risk_level: string;
  last_scanned_at?: string;
  current_version_id?: number;
  current_version_no?: number;
  content_hash?: string;
  archive_hash?: string;
  instance_count: number;
  created_at: string;
  updated_at: string;
}

export interface SkillVersion {
  id: number;
  skill_id: number;
  blob_id: number;
  version_no: number;
  source_type: string;
  content_hash: string;
  archive_hash: string;
  object_key: string;
  file_name: string;
  risk_level: string;
  created_at: string;
}

export interface SkillScanResult {
  id: number;
  blob_id: number;
  engine: string;
  risk_level: string;
  status: string;
  summary?: string;
  findings?: Record<string, unknown>;
  scanned_at?: string;
}

export interface InstanceSkill {
  id: number;
  instance_id: number;
  skill_id: number;
  skill_version_id?: number;
  source_type: string;
  install_path?: string;
  observed_hash?: string;
  status: string;
  last_seen_at?: string;
  removed_at?: string;
  skill?: Skill;
}
