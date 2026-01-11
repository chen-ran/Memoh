import type { Command } from 'commander'
import chalk from 'chalk'
import inquirer from 'inquirer'
import ora from 'ora'
import { table } from 'table'
import * as modelCore from '../../core/model'
import { formatError } from '../../utils'
import { getApiUrl } from '../../core/client'

export function modelCommands(program: Command) {
  program
    .command('list')
    .description('List all model configurations')
    .action(async () => {
      const spinner = ora('Fetching model list...').start()
      try {
        const models = await modelCore.listModels()
        spinner.succeed(chalk.green('Model List'))

        if (models.length === 0) {
          console.log(chalk.yellow('No model configurations found'))
          return
        }

        const tableData = [
          ['ID', 'Name', 'Model ID', 'Type', 'Client'],
          ...models.map((item) => [
            item.id.substring(0, 8) + '...',
            item.model.name || '-',
            item.model.modelId,
            item.model.type === 'embedding' ? chalk.yellow('embedding') : chalk.blue('chat'),
            item.model.clientType,
          ]),
        ]

        console.log(table(tableData))
      } catch (error) {
        spinner.fail(chalk.red('Operation failed'))
        if (error instanceof Error) {
          if (error.name === 'AbortError' || error.name === 'TimeoutError') {
            console.error(chalk.red('Connection timeout, please check:'))
            console.error(chalk.yellow('  1. Is the API server running?'))
            console.error(chalk.yellow('  2. Is the API URL correct?'))
            console.error(chalk.dim(`     Current config: ${getApiUrl()}`))
          } else {
            console.error(chalk.red('Error:'), error.message)
          }
        } else {
          console.error(chalk.red('Error:'), String(error))
        }
        process.exit(1)
      }
    })

  program
    .command('create')
    .description('Create model configuration')
    .option('-n, --name <name>', 'Model name')
    .option('-m, --model-id <modelId>', 'Model ID')
    .option('-u, --base-url <baseUrl>', 'API Base URL')
    .option('-k, --api-key <apiKey>', 'API Key')
    .option('-c, --client-type <clientType>', 'Client type (openai/anthropic/google)')
    .option('-t, --type <type>', 'Model type (chat/embedding)', 'chat')
    .option('-d, --dimensions <dimensions>', 'Embedding dimensions (required for embedding type)')
    .action(async (options) => {
      // const spinner = ora('Creating model configuration...').start()
      try {
        let { name, modelId, baseUrl, apiKey, clientType, type, dimensions } = options

        if (!name || !modelId || !baseUrl || !apiKey || !clientType) {
          const answers = await inquirer.prompt([
            {
              type: 'input',
              name: 'name',
              message: 'Model name:',
              when: !name,
            },
            {
              type: 'input',
              name: 'modelId',
              message: 'Model ID (e.g., gpt-4 or text-embedding-3-small):',
              when: !modelId,
            },
            {
              type: 'input',
              name: 'baseUrl',
              message: 'API Base URL:',
              default: 'https://api.openai.com/v1',
              when: !baseUrl,
            },
            {
              type: 'password',
              name: 'apiKey',
              message: 'API Key:',
              when: !apiKey,
              mask: '*',
            },
            {
              type: 'list',
              name: 'clientType',
              message: 'Client type:',
              choices: ['openai', 'anthropic', 'google'],
              default: 'openai',
              when: !clientType,
            },
            {
              type: 'list',
              name: 'type',
              message: 'Model type:',
              choices: ['chat', 'embedding'],
              default: 'chat',
              when: !type,
            },
          ])

          name = name || answers.name
          modelId = modelId || answers.modelId
          baseUrl = baseUrl || answers.baseUrl
          apiKey = apiKey || answers.apiKey
          clientType = clientType || answers.clientType
          type = type || answers.type
        }

        // If embedding type, dimensions is required
        if (type === 'embedding' && !dimensions) {
          const answer = await inquirer.prompt([
            {
              type: 'number',
              name: 'dimensions',
              message: 'Embedding dimensions (e.g., 1536):',
              validate: (value: number) => {
                if (value > 0) return true
                return 'Dimensions must be a positive integer'
              },
            },
          ])
          dimensions = answer.dimensions
        }

        // spinner.text = 'Creating model configuration...'

        const model = await modelCore.createModel({
          name,
          modelId,
          baseUrl,
          apiKey,
          clientType,
          type: type as 'chat' | 'embedding',
          dimensions: dimensions ? (typeof dimensions === 'number' ? dimensions : parseInt(dimensions)) : undefined,
        })

        // spinner.succeed(chalk.green('Model configuration created successfully'))
        console.log(chalk.blue(`Name: ${model.name}`))
        console.log(chalk.blue(`Model ID: ${model.modelId}`))
        console.log(chalk.blue(`Type: ${model.type || 'chat'}`))
        if (model.type === 'embedding' && model.dimensions) {
          console.log(chalk.blue(`Dimensions: ${model.dimensions}`))
        }
        console.log(chalk.blue(`ID: ${model.id}`))
      } catch (error) {
        // spinner.fail(chalk.red('Operation failed'))
        console.error(chalk.red(formatError(error)))
        process.exit(1)
      }
    })

  program
    .command('delete <id>')
    .description('Delete model configuration')
    .action(async (id) => {
      try {
        const { confirm } = await inquirer.prompt([
          {
            type: 'confirm',
            name: 'confirm',
            message: chalk.yellow(`Are you sure you want to delete model configuration ${id}?`),
            default: false,
          },
        ])

        if (!confirm) {
          console.log(chalk.yellow('Cancelled'))
          return
        }

        const spinner = ora('Deleting model configuration...').start()
        await modelCore.deleteModel(id)
        spinner.succeed(chalk.green('Model configuration deleted'))
      } catch (error) {
        console.error(chalk.red(formatError(error)))
        process.exit(1)
      }
    })

  program
    .command('get <id>')
    .description('Get model configuration details')
    .action(async (id) => {
      const spinner = ora('Fetching model configuration...').start()
      try {
        const model = await modelCore.getModel(id)
        spinner.succeed(chalk.green('Model Configuration'))
        console.log(chalk.blue(`ID: ${model.id}`))
        console.log(chalk.blue(`Name: ${model.name}`))
        console.log(chalk.blue(`Model ID: ${model.modelId}`))
        console.log(chalk.blue(`Type: ${model.type || 'chat'}`))
        if (model.type === 'embedding' && model.dimensions) {
          console.log(chalk.blue(`Dimensions: ${model.dimensions}`))
        }
        console.log(chalk.blue(`Base URL: ${model.baseUrl}`))
        console.log(chalk.blue(`Client Type: ${model.clientType}`))
        console.log(chalk.blue(`Created At: ${new Date(model.createdAt).toLocaleString('en-US')}`))
      } catch (error) {
        spinner.fail(chalk.red('Operation failed'))
        console.error(chalk.red(formatError(error)))
        process.exit(1)
      }
    })

  program
    .command('defaults')
    .description('View default model configurations')
    .action(async () => {
      const spinner = ora('Fetching default model configurations...').start()
      try {
        const defaults = await modelCore.getDefaultModels()
        spinner.stop()

        console.log(chalk.green.bold('Default Model Configurations:'))
        console.log()

        // Chat Model
        if (defaults.chat) {
          console.log(chalk.blue('💬 Chat Model:'))
          console.log(chalk.dim(`  Name: ${defaults.chat.name}`))
          console.log(chalk.dim(`  Model ID: ${defaults.chat.modelId}`))
          console.log(chalk.dim(`  ID: ${defaults.chat.id}`))
        } else {
          console.log(chalk.yellow('💬 Chat Model: Not configured'))
        }
        console.log()

        // Summary Model
        if (defaults.summary) {
          console.log(chalk.blue('📝 Summary Model:'))
          console.log(chalk.dim(`  Name: ${defaults.summary.name}`))
          console.log(chalk.dim(`  Model ID: ${defaults.summary.modelId}`))
          console.log(chalk.dim(`  ID: ${defaults.summary.id}`))
        } else {
          console.log(chalk.yellow('📝 Summary Model: Not configured'))
        }
        console.log()

        // Embedding Model
        if (defaults.embedding) {
          console.log(chalk.blue('🔍 Embedding Model:'))
          console.log(chalk.dim(`  Name: ${defaults.embedding.name}`))
          console.log(chalk.dim(`  Model ID: ${defaults.embedding.modelId}`))
          console.log(chalk.dim(`  ID: ${defaults.embedding.id}`))
        } else {
          console.log(chalk.yellow('🔍 Embedding Model: Not configured'))
        }
      } catch (error) {
        spinner.fail(chalk.red('Operation failed'))
        console.error(chalk.red(formatError(error)))
        process.exit(1)
      }
    })
}

