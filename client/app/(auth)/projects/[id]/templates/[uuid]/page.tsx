// Filename: app/(auth)/projects/[id]/templates/[uuid]/page.tsx
"use client";

import { useState, useEffect, useCallback } from 'react';
import { useParams } from 'next/navigation'; // FIX: Removed unused 'useRouter'
import Link from 'next/link';
import { toast } from "sonner";
import Editor from '@monaco-editor/react';
import * as api from '@/lib/api';

import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { ArrowLeft, Save } from 'lucide-react';

export default function TemplateEditorPage() {
  const params = useParams();
  
  const projectId = Array.isArray(params.id) ? params.id[0] : params.id;
  const templateUuid = Array.isArray(params.uuid) ? params.uuid[0] : params.uuid;

  // FIX: Removed unused 'template' state. We only need the string content.
  const [editedContent, setEditedContent] = useState<string>("");
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);

  const fetchTemplate = useCallback(async () => {
    if (!templateUuid) return;
    setIsLoading(true);
    try {
      const response = await api.getTemplateByUuid(templateUuid);
      // We don't need to store the object, just format it for the editor
      setEditedContent(JSON.stringify(response.data, null, 2));
    } catch (error) {
      console.error("Failed to fetch template", error);
      toast.error("Could not load template data.");
    } finally {
      setIsLoading(false);
    }
  }, [templateUuid]);

  useEffect(() => {
    fetchTemplate();
  }, [fetchTemplate]);

  const handleSave = async () => {
    if (!templateUuid) return;

    let parsedContent;
    try {
      parsedContent = JSON.parse(editedContent);
    } catch (error) {
      console.log(error)
      toast.error("Invalid JSON format. Please correct the errors before saving.");
      return;
    }

    setIsSaving(true);
    try {
      await api.updateTemplate(templateUuid, parsedContent);
      toast.success("Template saved successfully!");
    } catch (error) {
      console.error("Failed to save template", error);
      toast.error("Failed to save template.");
    } finally {
      setIsSaving(false);
    }
  };

  if (isLoading) {
    return (
      <div className="max-w-7xl mx-auto p-4 sm:p-6 lg:p-8 space-y-4">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-9 w-24" />
        <Skeleton className="h-[60vh] w-full" />
      </div>
    );
  }

  return (
    <div className="max-w-7xl mx-auto p-4 sm:p-6 lg:p-8">
      <div className="mb-4">
        <Link href={`/projects/${projectId}`} className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-4 w-4" />
          Back to Project
        </Link>
      </div>
      
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-2xl font-bold">Edit Template</h2>
        <Button onClick={handleSave} disabled={isSaving}>
          <Save className="mr-2 h-4 w-4" />
          {isSaving ? "Saving..." : "Save"}
        </Button>
      </div>

      <div className="border rounded-md overflow-hidden">
        <Editor
          height="70vh"
          language="json"
          value={editedContent}
          onChange={(value) => setEditedContent(value || "")}
          theme="vs-dark"
          options={{ minimap: { enabled: false } }}
        />
      </div>
    </div>
  );
}