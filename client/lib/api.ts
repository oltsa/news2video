// Filename: lib/api.ts
import axios, { AxiosResponse } from 'axios';

// --- Instantiation ---
const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_BASE_URL,
  withCredentials: true,
});

// --- Type Definitions ---

export interface User { id: string; organization_id: string; is_customer_admin: boolean; }
export interface Project { id: string; project_key: string; }
export interface ProjectDetail extends Project { api_key: string; }
export interface AssetItem { key: string; last_modified: string; size: number; is_dir: boolean; }
export interface UploadUrlRequest { filename: string; contentType: string; assetType: string; }
export interface UploadUrlResponse { url: string; }
export interface LoginCredentials { email: string; password: string; }
export interface LoginResponse { message: string; }
export interface LogoutResponse { message: string; }
export interface CreateFolderRequest { path: string; folderName: string; }
export type S3Config = object
export interface WebhookConfig { url: string; headers?: Record<string, string>; }
export type Connection = { id: string; } & ({ platform: 's3'; config: S3Config } | { platform: 'webhook'; config: WebhookConfig });
export type CreateConnectionRequest = | { platform: 's3'; config: S3Config } | { platform: 'webhook'; config: WebhookConfig };
export interface Template { uuid: string; id: string; is_system: boolean; }
export interface TemplateDetail { [key: string]: unknown; }
export interface CreateTemplateRequest { id: string; templateData: TemplateDetail;}
export interface RenderJobRequest { templateId: string; payload: Record<string, unknown>; backgroundImageUrl?: string; }
export interface RenderJobResponse { message: string; jobId: string; }

export type NullableString = { String: string; Valid: boolean; } | null;
export type NullableTime = { Time: string; Valid: boolean; } | null;

export interface Job {
  id: string;
  project_key: string;
  template_id: string;
  status: 'queued' | 'rendering' | 'complete' | 'failed';
  created_at: string;
  completed_at: NullableTime;
  error_message: NullableString;
  output_storage_key: NullableString; // Also updated this for consistency
  output_url?: string;
}

// --- API Functions ---

/**
 * Authenticates a user and establishes a session cookie.
 * @param credentials The user's email and password.
 * @returns A promise with a success message.
 */
export const login = (credentials: LoginCredentials): Promise<AxiosResponse<LoginResponse>> => api.post('/auth/login', credentials);

/**
 * Logs the user out by clearing the session cookie on the server.
 * @returns A promise with a success message.
 */
export const logout = (): Promise<AxiosResponse<LogoutResponse>> => api.post('/auth/logout');

/**
 * Fetches the profile of the currently authenticated user.
 * @returns A promise containing the user's data.
 */
export const getMe = (): Promise<AxiosResponse<User>> => api.get('/me');

/**
 * Fetches the list of projects accessible to the current user.
 * @returns A promise containing an array of projects.
 */
export const getProjects = (): Promise<AxiosResponse<Project[]>> => api.get<Project[]>('/projects');

/**
 * Fetches the detailed information for a single project by its ID.
 * @param projectId The UUID of the project.
 * @returns A promise containing the detailed project data.
 */
export const getProjectById = (projectId: string): Promise<AxiosResponse<ProjectDetail>> => api.get(`/projects/${projectId}`);

/**
 * Fetches the list of assets for a given project at a specific path.
 * @param projectId The UUID of the project.
 * @param path The sub-directory path to list.
 * @returns A promise containing the list of assets.
 */
export const listAssets = (projectId: string, path: string): Promise<AxiosResponse<AssetItem[]>> => api.get(`/projects/${projectId}/assets`, { params: { path } });

/**
 * Deletes a specific asset from a project.
 * @param projectId The UUID of the project.
 * @param assetKey The full key of the asset to delete (e.g., "logos/my-logo.png").
 * @returns A promise that resolves on successful deletion.
 */
export const deleteAsset = (projectId: string, assetKey: string): Promise<AxiosResponse<void>> => api.delete(`/projects/${projectId}/assets`, { params: { key: assetKey } });

/**
 * Gets a pre-signed URL for uploading a file directly to S3.
 * @param projectId The UUID of the project.
 * @param data The details of the file to be uploaded.
 * @returns A promise containing the pre-signed URL.
 */
export const getUploadUrl = (projectId: string, data: UploadUrlRequest): Promise<AxiosResponse<UploadUrlResponse>> => api.post(`/projects/${projectId}/assets/upload-url`, data);

/**
 * Creates a new folder at a specific path within a project's assets.
 * @param projectId The UUID of the project.
 * @param data The path and name for the new folder.
 * @returns A promise that resolves on successful creation.
 */
export const createFolder = (projectId: string, data: CreateFolderRequest): Promise<AxiosResponse<void>> => api.post(`/projects/${projectId}/assets/folder`, data);

/**
 * Deletes a folder and all its contents from a project.
 * @param projectId The UUID of the project.
 * @param path The full path of the folder to delete.
 * @returns A promise that resolves on successful deletion.
 */
export const deleteFolder = (projectId: string, path: string): Promise<AxiosResponse<void>> => api.delete(`/projects/${projectId}/assets/folder`, { params: { path } });

/**
 * Fetches the list of connections for a given project.
 * @param projectId The UUID of the project.
 * @returns A promise containing the list of connections.
 */
export const listConnections = (projectId: string): Promise<AxiosResponse<Connection[]>> => api.get(`/projects/${projectId}/connections`);

/**
 * Creates a new connection for a project.
 * @param projectId The UUID of the project.
 * @param data The connection platform and configuration.
 * @returns A promise with the newly created connection data.
 */
export const createConnection = (projectId: string, data: CreateConnectionRequest): Promise<AxiosResponse<Connection>> => api.post(`/projects/${projectId}/connections`, data);

/**
 * Deletes a specific connection.
 * @param connectionId The UUID of the connection to delete.
 * @returns A promise that resolves on successful deletion.
 */
export const deleteConnection = (connectionId: string): Promise<AxiosResponse<void>> => api.delete(`/connections/${connectionId}`);

/**
 * Fetches the list of templates for a given project.
 * @param projectId The UUID of the project.
 * @returns A promise containing the list of templates.
 */
export const listTemplates = (projectId: string): Promise<AxiosResponse<Template[]>> => api.get(`/projects/${projectId}/templates`);

/**
 * Fetches the full JSON definition of a single template.
 * @param templateUuid The UUID of the template.
 * @returns A promise containing the raw template JSON.
 */
export const getTemplateByUuid = (templateUuid: string): Promise<AxiosResponse<TemplateDetail>> => api.get(`/templates/${templateUuid}`);

/**
 * Submits a new render job from the dashboard.
 * @param projectId The UUID of the project.
 * @param data The render job request payload.
 * @returns A promise containing the job submission response.
 */
export const createRenderJob = (projectId: string, data: RenderJobRequest): Promise<AxiosResponse<RenderJobResponse>> => api.post(`/projects/${projectId}/jobs`, data);

/**
 * Updates a template's JSON content.
 * @param templateUuid The UUID of the template to update.
 * @param data The new, full JSON object for the template.
 * @returns A promise that resolves on successful update.
 */
export const updateTemplate = (templateUuid: string, data: TemplateDetail): Promise<AxiosResponse<void>> => {
  return api.put(`/templates/${templateUuid}`, data);
};

/**
 * Creates a new custom template for a project.
 * @param projectId The UUID of the project.
 * @param data The ID and JSON content of the new template.
 * @returns A promise containing the newly created template data.
 */
export const createTemplate = (projectId: string, data: CreateTemplateRequest): Promise<AxiosResponse<Template>> => {
  return api.post(`/projects/${projectId}/templates`, data);
};

/**
 * Deletes a custom project template.
 * @param templateUuid The UUID of the template to delete.
 * @returns A promise that resolves on successful deletion.
 */
export const deleteTemplate = (templateUuid: string): Promise<AxiosResponse<void>> => {
  return api.delete(`/templates/${templateUuid}`);
};

/**
 * Fetches all jobs submitted by the current user.
 * @param limit Optional limit for fetching only the most recent jobs.
 * @returns A promise containing the list of jobs.
 */
export const listMyJobs = (limit?: number): Promise<AxiosResponse<Job[]>> => {
  return api.get('/jobs', {
    params: limit ? { limit } : {},
  });
};

export default api;