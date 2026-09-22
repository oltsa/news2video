// Filename: app/(auth)/projects/[id]/page.tsx
"use client";

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import * as api from '@/lib/api';
import { ProjectDetail } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { ArrowLeft, Copy } from 'lucide-react';
import { AssetList } from '@/components/assets/AssetList';
import { TemplateList } from '@/components/templates/TemplateList';
import { ConnectionList } from '@/components/connections/ConnectionList';

export default function ProjectDetailPage() {
  const params = useParams();
  const id = Array.isArray(params.id) ? params.id[0] : params.id;

  const [project, setProject] = useState<ProjectDetail | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isCopied, setIsCopied] = useState(false);

  useEffect(() => {
    if (id) {
      const fetchProject = async () => {
        setIsLoading(true);
        try {
          const response = await api.getProjectById(id);
          setProject(response.data);
        } catch (err) {
          setError('Failed to load project details.');
          console.error(err);
        } finally {
          setIsLoading(false);
        }
      };
      fetchProject();
    }
  }, [id]);

  const handleCopy = () => {
    if (project?.api_key) {
      navigator.clipboard.writeText(project.api_key);
      setIsCopied(true);
      setTimeout(() => setIsCopied(false), 2000);
    }
  };

  if (isLoading) {
    return <ProjectDetailSkeleton />;
  }

  if (error) {
    return <div className="text-red-500 p-8">{error}</div>;
  }

  if (!project) {
    return <div className="p-8">Project not found.</div>;
  }

  return (
    <div className="max-w-7xl mx-auto p-4 sm:p-6 lg:p-8 space-y-8">
      <div>
        <Link href="/dashboard" className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground mb-4">
          <ArrowLeft className="h-4 w-4" />
          Back to Dashboard
        </Link>

        <Card>
          <CardHeader>
            <CardTitle className="text-2xl">{project.project_key}</CardTitle>
            <CardDescription>Project ID: {project.id}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-2 max-w-lg">
              <Label htmlFor="api-key">API Key</Label>
              <div className="flex items-center gap-2">
                <Input id="api-key" value={project.api_key} readOnly />
                <Button variant="outline" size="icon" onClick={handleCopy}>
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
              {isCopied && <p className="text-sm text-green-600">Copied to clipboard!</p>}
            </div>
          </CardContent>
        </Card>
      </div>

      {id && <AssetList projectId={id} />}

      {id && <TemplateList projectId={id} />}

      {id && <ConnectionList projectId={id} />}
    </div>
  );
}

function ProjectDetailSkeleton() {
  return (
    <div className="max-w-7xl mx-auto p-4 sm:p-6 lg:p-8">
      <Skeleton className="h-6 w-48 mb-4" />
      <Card>
        <CardHeader>
          <Skeleton className="h-8 w-1/2" />
          <Skeleton className="h-4 w-3/4 mt-2" />
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="space-y-2">
            <Skeleton className="h-5 w-24" />
            <div className="flex items-center gap-2">
              <Skeleton className="h-9 grow" />
              <Skeleton className="h-9 w-9" />
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}