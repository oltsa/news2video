// Filename: app/(auth)/projects/[id]/templates/new/page.tsx
"use client";

import { useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { toast } from "sonner";
import Editor from '@monaco-editor/react';
import * as api from '@/lib/api';
import axios from 'axios';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { ArrowLeft, Save, Upload, FileJson } from 'lucide-react';

// +++ FIX: Updated boilerplate to the new schema with font/boundingBox +++
const boilerplateTemplate = {
    "name": "new_custom_template",
    "schema_version": "0.2.0",
    "canvas": { "width": 1080, "height": 1920 },
    "duration_sec": 12,
    "fps": 30,
    "layers": [
        {
            "name": "background",
            "type": "image",
            "file": "{{SYSTEM_ASSETS}}/backgrounds/default.png",
            "start_sec": 0,
            "end_sec": 12,
            "motion": { "type": "drift", "speed": "slow", "zoom_range": [1.0, 1.05] }
        },
        {
            "name": "title",
            "type": "text",
            "text_content": "{{.title}}",
            "pos": { "x": 540, "y": 960 },
            "gravity": "center",
            "start_sec": 0.5,
            "end_sec": 11.5,
            "font": {
                "file": "{{SYSTEM_ASSETS}}/fonts/DIN Alternate Bold.ttf",
                "size": 100,
                "color": "#FFFFFF"
            },
            "boundingBox": {
                "width": 960
            },
            "shadow": { "opacity": 0.65, "blur": 15, "dx": 4, "dy": 4 }
        }
    ]
};

export default function NewTemplatePage() {
  const params = useParams();
  const router = useRouter();
  const projectId = Array.isArray(params.id) ? params.id[0] : params.id;

  const [templateId, setTemplateId] = useState('');
  const [jsonContent, setJsonContent] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState('');

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (event) => {
      try {
        const content = event.target?.result as string;
        const parsed = JSON.parse(content);
        setJsonContent(JSON.stringify(parsed, null, 2));
        if (parsed.name && typeof parsed.name === 'string') {
          setTemplateId(parsed.name);
        }
        toast.success("Template file loaded successfully.");
      } catch {
        toast.error("Failed to parse JSON file.");
      }
    };
    reader.readAsText(file);
  };

  const loadBoilerplate = () => {
    setJsonContent(JSON.stringify(boilerplateTemplate, null, 2));
    setTemplateId('new_custom_template');
  };

  const handleSave = async () => {
    if (!projectId) {
      toast.error("Project ID is missing. Cannot save.");
      return;
    }

    if (!templateId.trim()) {
      setError("Template ID cannot be empty.");
      return;
    }
    
    let parsedContent;
    try {
      parsedContent = JSON.parse(jsonContent);
    } catch {
      setError("Invalid JSON format.");
      toast.error("Invalid JSON format. Please correct the errors before saving.");
      return;
    }

    setIsSaving(true);
    setError('');
    try {
      await api.createTemplate(projectId, { id: templateId, templateData: parsedContent });
      toast.success("Template created successfully!");
      router.push(`/projects/${projectId}`);
    } catch (err: unknown) {
      let errorMessage = "Failed to create template.";
      if (axios.isAxiosError<{ error: string }>(err) && err.response?.data?.error) {
        errorMessage = err.response.data.error;
      }
      setError(errorMessage);
      toast.error(errorMessage);
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="max-w-7xl mx-auto p-4 sm:p-6 lg:p-8 space-y-6">
      <div>
        <Link href={`/projects/${projectId || ''}`} className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground mb-4">
          <ArrowLeft className="h-4 w-4" />
          Back to Project
        </Link>
        <div className="flex justify-between items-center">
          <h2 className="text-2xl font-bold">New Custom Template</h2>
          <Button onClick={handleSave} disabled={isSaving || !templateId || !jsonContent}>
            <Save className="mr-2 h-4 w-4" />
            {isSaving ? "Saving..." : "Save Template"}
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Template Configuration</CardTitle>
          <CardDescription>
            Provide a unique ID for your template. This ID will be used in API calls.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Label htmlFor="template-id">Template ID</Label>
          <Input
            id="template-id"
            value={templateId}
            onChange={(e) => setTemplateId(e.target.value)}
            placeholder="e.g., my_custom_promo"
            className="max-w-sm"
          />
        </CardContent>
      </Card>
      
      <Card>
        <CardHeader>
          <CardTitle>Template Content</CardTitle>
          <CardDescription>
            Load content from a boilerplate or upload your own JSON file.
          </CardDescription>
          <div className="flex gap-2 pt-4">
            <Button variant="outline" onClick={loadBoilerplate}>
              <FileJson className="mr-2 h-4 w-4" />
              Start with Boilerplate
            </Button>
            <Button asChild variant="outline">
              <Label htmlFor="file-upload">
                <Upload className="mr-2 h-4 w-4" />
                Upload from File
                <input id="file-upload" type="file" accept=".json" className="sr-only" onChange={handleFileChange} />
              </Label>
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="border rounded-md overflow-hidden">
            <Editor
              height="60vh"
              language="json"
              value={jsonContent}
              onChange={(value) => setJsonContent(value || "")}
              theme="vs-dark"
              options={{ minimap: { enabled: false } }}
            />
          </div>
          {error && <p className="text-red-500 mt-2">{error}</p>}
        </CardContent>
      </Card>
    </div>
  );
}