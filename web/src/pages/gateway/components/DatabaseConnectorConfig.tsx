import React, { useState } from 'react';
import { Card, CardBody, CardHeader, Tabs, Tab, Input, Select, SelectItem, Switch, Button } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useTranslation } from 'react-i18next';
import { SnowflakeConfigForm } from './SnowflakeConfigForm';

interface DatabaseConnectorConfigProps {
  config: any;
  onChange: (config: any) => void;
}

export function DatabaseConnectorConfig({ config, onChange }: DatabaseConnectorConfigProps) {
  const { t } = useTranslation();
  const [dbType, setDbType] = useState(config?.type || 'none');
  
  const handleChange = (field: string, value: any) => {
    const updatedConfig = {
      ...config,
      [field]: value
    };
    onChange(updatedConfig);
  };

  const handleDbTypeChange = (type: string) => {
    setDbType(type);
    
    // Create a new config with the selected type
    const newConfig = {
      ...config,
      type: type
    };
    
    // Clear previous database-specific configs if changing types
    if (type === 'none') {
      newConfig.snowflake = undefined;
      newConfig.postgres = undefined;
      // Clear other database types as they're added
    }
    
    onChange(newConfig);
  };

  const handleSnowflakeConfigChange = (snowflakeConfig: any) => {
    onChange({
      ...config,
      type: 'snowflake',
      snowflake: snowflakeConfig
    });
  };

  return (
    <Card className="mt-4">
      <CardHeader>
        <div className="flex items-center gap-2">
          <Icon icon="lucide:database" className="text-primary" />
          <h3 className="text-lg font-semibold">{t('database.connector_title')}</h3>
        </div>
      </CardHeader>
      <CardBody>
        <div className="space-y-4">
          <Select
            label={t('database.type')}
            selectedKeys={[dbType]}
            onChange={(e) => handleDbTypeChange(e.target.value)}
            aria-label={t('database.type')}
          >
            <SelectItem key="none">{t('database.none')}</SelectItem>
            <SelectItem key="snowflake">Snowflake</SelectItem>
            {/* Add other database types here as they're implemented */}
          </Select>

          {dbType !== 'none' && (
            <>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Switch
                  isSelected={config?.enableAPI}
                  onValueChange={(value) => handleChange('enableAPI', value)}
                >
                  {t('database.enable_api')}
                </Switch>
                <Switch
                  isSelected={config?.enableLLM}
                  onValueChange={(value) => handleChange('enableLLM', value)}
                >
                  {t('database.enable_llm')}
                </Switch>
              </div>

              {config?.enableAPI && (
                <Input
                  label={t('database.api_prefix')}
                  value={config?.apiPrefix || ''}
                  onChange={(e) => handleChange('apiPrefix', e.target.value)}
                  placeholder="/api/db"
                  helperText={t('database.api_prefix_helper')}
                />
              )}

              <Tabs aria-label="Database Configuration">
                {dbType === 'snowflake' && (
                  <Tab key="snowflake" title="Snowflake">
                    <div className="pt-4">
                      <SnowflakeConfigForm 
                        config={config?.snowflake || {}} 
                        onChange={handleSnowflakeConfigChange} 
                      />
                    </div>
                  </Tab>
                )}
                {/* Add tabs for other database types as they're implemented */}
              </Tabs>
            </>
          )}
        </div>
      </CardBody>
    </Card>
  );
}
