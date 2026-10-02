export interface SettingDefinition {
  key: string;
  type: "bool" | "int" | "string";
  default: string;
  group: string;
  label: string;
  public: boolean;
  is_flag: boolean;
}

export interface SettingView extends SettingDefinition {
  value: string;
  updated_by?: string;
  updated_at?: string;
}
